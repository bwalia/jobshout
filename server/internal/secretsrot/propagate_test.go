package secretsrot

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// ── kubeconfig ─────────────────────────────────────────────────────────────

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func selfSignedClientCert(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "secretsrot"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
}

func TestParseKubeconfig_ClientCertAndCA(t *testing.T) {
	var sawClientCert bool
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawClientCert = r.TLS != nil && len(r.TLS.PeerCertificates) == 1
		_, _ = w.Write([]byte(`{"metadata":{"resourceVersion":"7"}}`))
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	srv.StartTLS()
	defer srv.Close()

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	certPEM, keyPEM := selfSignedClientCert(t)
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: default
clusters:
- name: default
  cluster:
    server: %s
    certificate-authority-data: %s
users:
- name: default
  user:
    client-certificate-data: %s
    client-key-data: %s
contexts:
- name: default
  context: {cluster: default, user: default}
`, srv.URL, b64(string(caPEM)), b64(certPEM), b64(keyPEM))

	kc, err := ParseKubeconfig([]byte(kubeconfig), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	info, err := kc.GetSecretInfo(context.Background(), "int", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !sawClientCert || info.ResourceVersion != "7" {
		t.Fatalf("clientCert=%v info=%+v", sawClientCert, info)
	}
}

func TestParseKubeconfig_TokenAndCurrentContext(t *testing.T) {
	var auth string
	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"metadata":{"resourceVersion":"1"}}`))
	}))
	defer good.Close()
	kubeconfig := fmt.Sprintf(`current-context: k3s1
clusters:
- name: other
  cluster: {server: "https://127.0.0.1:1"}
- name: k3s1
  cluster:
    server: %s
    insecure-skip-tls-verify: true
users:
- name: other
  user: {token: wrong}
- name: sa
  user: {token: sa-token}
contexts:
- name: other
  context: {cluster: other, user: other}
- name: k3s1
  context: {cluster: k3s1, user: sa}
`, good.URL)
	kc, err := ParseKubeconfig([]byte(kubeconfig), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if kc.Server() != good.URL {
		t.Fatalf("server %s", kc.Server())
	}
	if _, err := kc.GetSecretInfo(context.Background(), "int", "x"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer sa-token" {
		t.Fatalf("auth %q", auth)
	}
}

func TestParseKubeconfig_ExecRejected(t *testing.T) {
	_, err := ParseKubeconfig([]byte(`current-context: c
clusters: [{name: c, cluster: {server: "https://x"}}]
users: [{name: u, user: {exec: {command: aws}}}]
contexts: [{name: c, context: {cluster: c, user: u}}]
`), 0)
	if err == nil || !strings.Contains(err.Error(), "exec credential plugin") {
		t.Fatalf("err %v", err)
	}
}

func TestLoadKubeconfigBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "k3s1.yaml"), []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := LoadKubeconfigBytes(dir, "k3s1")
	if err != nil || string(b) != "from-file" {
		t.Fatalf("%q %v", b, err)
	}
	t.Setenv("SECRETS_ROT_KUBECONFIG_K3S1", "from-env")
	b, _ = LoadKubeconfigBytes(dir, "k3s1")
	if string(b) != "from-env" {
		t.Fatalf("env should win, got %q", b)
	}
	for _, bad := range []string{"../etc", "K3S1", "a/b", ""} {
		if _, err := LoadKubeconfigBytes(dir, bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if _, err := LoadKubeconfigBytes(dir, "missing"); err == nil || !strings.Contains(err.Error(), "SECRETS_ROT_KUBECONFIG_MISSING") {
		t.Fatalf("err %v", err)
	}
}

// ── fake world: k8s + GitHub + Ring Promoter + Vault ─────────────────────

type fakeWorld struct {
	t  *testing.T
	mu sync.Mutex

	writes []string // "METHOD path" for every non-GET request on every server

	// k8s
	k8s        *httptest.Server
	secrets    map[string]*fakeSecret // ns/name
	esos       map[string]*fakeESO
	noESO      bool
	esoReadyAt int // ES becomes Ready after this many GETs post-annotation (<0 never)
	patches    map[string]map[string]any
	deploys    map[string][]map[string]any // ns → deployment items

	// GitHub
	gh       *httptest.Server
	ghPub    *[32]byte
	ghPriv   *[32]byte
	ghPut    map[string]string // path → decrypted
	ghRepoID int

	// Ring Promoter
	rp          *httptest.Server
	rpJobs      map[string][]string // job id → status sequence
	rpHealthy   map[string]bool
	rpBodies    []map[string]any
	rpFailStart int

	// Vault
	vault        *httptest.Server
	vaultVersion int
	vaultData    map[string]any
	vaultWritten map[string]any
}

type fakeSecret struct {
	rv   int
	helm bool
	data map[string]string
}

type fakeESO struct {
	target    string
	annotated int64
	gets      int
}

func newFakeWorld(t *testing.T) *fakeWorld {
	w := &fakeWorld{
		t:            t,
		secrets:      map[string]*fakeSecret{},
		esos:         map[string]*fakeESO{},
		patches:      map[string]map[string]any{},
		deploys:      map[string][]map[string]any{},
		ghPut:        map[string]string{},
		ghRepoID:     42,
		rpJobs:       map[string][]string{},
		rpHealthy:    map[string]bool{"int": true, "test": true, "acc": true, "prod": true},
		vaultVersion: 4,
		vaultData:    map[string]any{"password": "old-password-value", "user": "app"},
	}
	w.ghPub, w.ghPriv, _ = box.GenerateKey(rand.Reader)
	w.k8s = httptest.NewServer(http.HandlerFunc(w.serveK8s))
	w.gh = httptest.NewServer(http.HandlerFunc(w.serveGH))
	w.rp = httptest.NewServer(http.HandlerFunc(w.serveRP))
	w.vault = httptest.NewServer(http.HandlerFunc(w.serveVault))
	t.Cleanup(func() { w.k8s.Close(); w.gh.Close(); w.rp.Close(); w.vault.Close() })
	t.Setenv("SECRETS_ROT_KUBECONFIG_K3S1", fmt.Sprintf(`current-context: c
clusters: [{name: c, cluster: {server: %q}}]
users: [{name: u, user: {token: kube-token}}]
contexts: [{name: c, context: {cluster: c, user: u}}]
`, w.k8s.URL))
	return w
}

func (w *fakeWorld) cfg() Config {
	return Config{
		Addr: w.vault.URL, Token: "vault-token", Timeout: 5 * time.Second,
		KubeconfigDir: w.t.TempDir(),
		GitHubToken:   "gh-token", GitHubAPI: w.gh.URL,
		RPURL: w.rp.URL, RPToken: "rp-token", RPProdPassword: fakeProdPW,
		ESOTimeout: 300 * time.Millisecond, ESOPollInterval: 10 * time.Millisecond,
		RPTimeout: 2 * time.Second, RPPollInterval: 5 * time.Millisecond,
	}
}

func (w *fakeWorld) record(r *http.Request) {
	if r.Method != http.MethodGet {
		w.writes = append(w.writes, r.Method+" "+r.URL.Path)
	}
}

func writeJSON(rw http.ResponseWriter, code int, v any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	_ = json.NewEncoder(rw).Encode(v)
}

func (w *fakeWorld) serveK8s(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.record(r)
	if r.Header.Get("Authorization") != "Bearer kube-token" {
		writeJSON(rw, 401, map[string]any{"message": "unauthorized"})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 6 && parts[0] == "api" && parts[4] == "secrets":
		key := parts[3] + "/" + parts[5]
		s := w.secrets[key]
		if s == nil {
			writeJSON(rw, 404, map[string]any{"message": "not found"})
			return
		}
		if r.Method == http.MethodPatch {
			if ct := r.Header.Get("Content-Type"); ct != "application/merge-patch+json" {
				w.t.Errorf("patch content-type %q", ct)
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.patches[key] = body
			if d, ok := body["data"].(map[string]any); ok {
				for k, v := range d {
					s.data[k] = v.(string)
				}
			}
			s.rv++
		}
		meta := map[string]any{"resourceVersion": fmt.Sprint(s.rv)}
		if s.helm {
			meta["labels"] = map[string]string{"app.kubernetes.io/managed-by": "Helm"}
			meta["annotations"] = map[string]string{"meta.helm.sh/release-name": "jobshout"}
		}
		writeJSON(rw, 200, map[string]any{"metadata": meta, "data": s.data})
	case len(parts) == 2 && parts[0] == "apis" && parts[1] == "external-secrets.io":
		if w.noESO {
			writeJSON(rw, 404, map[string]any{"message": "not found"})
			return
		}
		writeJSON(rw, 200, map[string]any{})
	case len(parts) == 7 && parts[1] == "external-secrets.io" && parts[5] == "externalsecrets":
		if w.noESO || parts[2] != "v1beta1" {
			writeJSON(rw, 404, map[string]any{"message": "not found"})
			return
		}
		key := parts[4] + "/" + parts[6]
		es := w.esos[key]
		if es == nil {
			writeJSON(rw, 404, map[string]any{"message": "not found"})
			return
		}
		if r.Method == http.MethodPatch {
			var body struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			fmt.Sscanf(body.Metadata.Annotations["force-sync"], "%d", &es.annotated)
			es.gets = 0
			writeJSON(rw, 200, map[string]any{})
			return
		}
		status := map[string]any{}
		if es.annotated > 0 {
			es.gets++
			if w.esoReadyAt >= 0 && es.gets > w.esoReadyAt {
				// ESO re-synced: bump the target Secret once.
				if ts := w.secrets[parts[4]+"/"+es.target]; ts != nil && es.gets == w.esoReadyAt+1 {
					ts.rv++
				}
				status = map[string]any{
					"refreshTime": time.Unix(es.annotated, 0).UTC().Format(time.RFC3339),
					"conditions":  []map[string]any{{"type": "Ready", "status": "True", "reason": "SecretSynced"}},
				}
			} else {
				status = map[string]any{"conditions": []map[string]any{{"type": "Ready", "status": "False", "reason": "SecretSyncedError", "message": "could not get secret data from provider"}}}
			}
		}
		writeJSON(rw, 200, map[string]any{
			"metadata": map[string]any{"name": parts[6], "resourceVersion": "1"},
			"spec":     map[string]any{"target": map[string]any{"name": es.target}},
			"status":   status,
		})
	case len(parts) == 6 && parts[0] == "apis" && parts[1] == "apps" && parts[5] == "deployments":
		writeJSON(rw, 200, map[string]any{"items": w.deploys[parts[4]]})
	default:
		writeJSON(rw, 404, map[string]any{"message": "no route " + r.URL.Path})
	}
}

func (w *fakeWorld) serveGH(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.record(r)
	if r.Header.Get("Authorization") != "Bearer gh-token" {
		writeJSON(rw, 401, map[string]any{"message": "Bad credentials"})
		return
	}
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/secrets/public-key"):
		writeJSON(rw, 200, map[string]any{"key_id": "kid-1", "key": base64.StdEncoding.EncodeToString(w.ghPub[:])})
	case r.Method == http.MethodGet && strings.Count(p, "/") == 3 && strings.HasPrefix(p, "/repos/"):
		writeJSON(rw, 200, map[string]any{"id": w.ghRepoID})
	case r.Method == http.MethodPut && strings.Contains(p, "/secrets/"):
		var body struct {
			EncryptedValue string `json:"encrypted_value"`
			KeyID          string `json:"key_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.KeyID != "kid-1" {
			w.t.Errorf("key_id %q", body.KeyID)
		}
		sealed, _ := base64.StdEncoding.DecodeString(body.EncryptedValue)
		plain, ok := box.OpenAnonymous(nil, sealed, w.ghPub, w.ghPriv)
		if !ok {
			w.t.Errorf("sealed box did not open for %s", p)
		}
		w.ghPut[p] = string(plain)
		rw.WriteHeader(201)
	default:
		writeJSON(rw, 404, map[string]any{"message": "Not Found"})
	}
}

func (w *fakeWorld) serveRP(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.record(r)
	if r.Header.Get("Authorization") != "Bearer rp-token" {
		writeJSON(rw, 401, map[string]any{"error": "missing or invalid bearer token"})
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodPost && len(parts) == 6 && parts[5] == "restart":
		if r.URL.Query().Get("async") != "1" {
			w.t.Errorf("restart without async=1")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["_ring"] = parts[4]
		w.rpBodies = append(w.rpBodies, body)
		if w.rpFailStart > 0 {
			writeJSON(rw, w.rpFailStart, map[string]any{"error": "nothing to restart"})
			return
		}
		id := fmt.Sprintf("job-%d", len(w.rpBodies))
		if _, ok := w.rpJobs[id]; !ok {
			w.rpJobs[id] = []string{"pending", "running", "success"}
		}
		writeJSON(rw, 202, map[string]any{"job_id": id})
	case r.Method == http.MethodGet && len(parts) == 5 && parts[3] == "jobs":
		seq := w.rpJobs[parts[4]]
		if len(seq) == 0 {
			writeJSON(rw, 404, map[string]any{"error": "job not found"})
			return
		}
		st := seq[0]
		if len(seq) > 1 {
			w.rpJobs[parts[4]] = seq[1:]
		}
		out := map[string]any{"id": parts[4], "action": "restart", "status": st}
		if st == "failed" {
			out["error"] = "rollout timed out"
		}
		writeJSON(rw, 200, out)
	case r.Method == http.MethodGet && len(parts) == 4 && parts[3] == "rings":
		var rings []map[string]any
		for _, n := range []string{"int", "test", "acc", "prod"} {
			rings = append(rings, map[string]any{"ring": map[string]any{"name": n}, "healthy": w.rpHealthy[n]})
		}
		writeJSON(rw, 200, map[string]any{"rings": rings})
	default:
		writeJSON(rw, 404, map[string]any{"error": "no route"})
	}
}

func (w *fakeWorld) serveVault(rw http.ResponseWriter, r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.record(r)
	switch {
	case strings.HasSuffix(r.URL.Path, "/v1/sys/health"):
		writeJSON(rw, 200, map[string]any{"initialized": true})
	case strings.Contains(r.URL.Path, "/metadata/"):
		writeJSON(rw, 200, map[string]any{"data": map[string]any{"current_version": w.vaultVersion}})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/data/"):
		writeJSON(rw, 200, map[string]any{"data": map[string]any{"data": w.vaultData, "metadata": map[string]any{"version": w.vaultVersion}}})
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/data/"):
		var body struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.vaultWritten = body.Data
		w.vaultData = body.Data
		w.vaultVersion++
		writeJSON(rw, 200, map[string]any{"data": map[string]any{"version": w.vaultVersion}})
	default:
		writeJSON(rw, 200, map[string]any{})
	}
}

func mustProp(t *testing.T, in PropagationInput) Propagation {
	t.Helper()
	p, err := ParsePropagation(in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func deployment(name string, spec map[string]any) map[string]any {
	return map[string]any{"metadata": map[string]any{"name": name}, "spec": map[string]any{"template": map[string]any{"spec": spec}}}
}

// ── k8s Secret patch ───────────────────────────────────────────────────────

func TestSecretPatchBody_OnlyRotatedKeysBase64(t *testing.T) {
	body := SecretPatchBody(map[string]string{"password": fakeValue})
	data := body["data"].(map[string]string)
	if len(data) != 1 || data["password"] != b64(fakeValue) {
		t.Fatalf("%#v", body)
	}
	if len(body) != 1 {
		t.Fatalf("patch must only carry data: %#v", body)
	}
}

func TestPropagate_K8sSecretPatchKeepsOtherKeys(t *testing.T) {
	w := newFakeWorld(t)
	w.secrets["int/jobshout-cms"] = &fakeSecret{rv: 10, data: map[string]string{"OTHER": b64("keep"), "password": b64("old")}}
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "jobshout/int/cms", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", K8sSecrets: "int/jobshout-cms"}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	patch := w.patches["int/jobshout-cms"]["data"].(map[string]any)
	if len(patch) != 1 {
		t.Fatalf("patch touched other keys: %#v", patch)
	}
	newVal := w.vaultWritten["password"].(string)
	if patch["password"] != b64(newVal) {
		t.Fatal("patched value is not base64 of the vault value")
	}
	if w.secrets["int/jobshout-cms"].data["OTHER"] != b64("keep") {
		t.Fatal("other key lost")
	}
	tg := out.Result.Propagation.Targets
	if len(tg) != 1 || tg[0].Status != "completed" || tg[0].ResourceVersion != "11" {
		t.Fatalf("targets %#v", tg)
	}
	// retire_old follows propagation
	var order []string
	for _, p := range out.Phases {
		order = append(order, p.Key)
	}
	if strings.Join(order, ",") != "preflight,read,write_new,dual_window,verify,k8s_secret,retire_old,audit" {
		t.Fatalf("phase order %v", order)
	}
}

func TestPropagate_HelmManagedSecretRefused(t *testing.T) {
	w := newFakeWorld(t)
	w.secrets["int/jobshout-secrets"] = &fakeSecret{rv: 3, helm: true, data: map[string]string{}}
	opt := RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", K8sSecrets: "int/jobshout-secrets"}),
	}
	_, err := Execute(context.Background(), w.cfg(), opt, nil)
	if err == nil || !strings.Contains(err.Error(), "Helm-managed") {
		t.Fatalf("err %v", err)
	}
	if len(w.writes) != 0 {
		t.Fatalf("live run must stop before any write: %v", w.writes)
	}
	// plan: warning only
	opt.Mode = "plan"
	out, err := Execute(context.Background(), w.cfg(), opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(out.Result.Warnings, "|"), "Helm-managed") {
		t.Fatalf("warnings %v", out.Result.Warnings)
	}
}

// ── ExternalSecret ─────────────────────────────────────────────────────────

func TestPropagate_ExternalSecretForceSyncSuccess(t *testing.T) {
	w := newFakeWorld(t)
	w.esoReadyAt = 2
	w.secrets["int/jobshout-secrets"] = &fakeSecret{rv: 5, data: map[string]string{}}
	w.esos["int/jobshout-secrets"] = &fakeESO{target: "jobshout-secrets"}
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "jobshout/int/config", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", ExternalSecrets: "int/jobshout-secrets"}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.esos["int/jobshout-secrets"].annotated == 0 {
		t.Fatal("force-sync annotation not set")
	}
	tg := out.Result.Propagation.Targets[0]
	if tg.Status != "completed" || tg.ResourceVersion != "6" {
		t.Fatalf("target %#v", tg)
	}
}

func TestPropagate_ExternalSecretTimeout(t *testing.T) {
	w := newFakeWorld(t)
	w.esoReadyAt = -1
	w.secrets["int/jobshout-secrets"] = &fakeSecret{rv: 5, data: map[string]string{}}
	w.esos["int/jobshout-secrets"] = &fakeESO{target: "jobshout-secrets"}
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "jobshout/int/config", Keys: []string{"password"}, RetireOld: true,
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", ExternalSecrets: "int/jobshout-secrets"}),
	}, nil)
	if err == nil {
		t.Fatal("expected timeout")
	}
	msg := err.Error()
	for _, want := range []string{"external_secret", "not synced within", "Vault secret/jobshout/int/config is at version 5", "SecretSyncedError"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	for _, p := range out.Phases {
		if p.Key == "retire_old" && p.Status != "skipped" {
			t.Fatalf("retire_old must not run after a failed propagation: %+v", p)
		}
	}
}

func TestPropagate_NoExternalSecretsOperator(t *testing.T) {
	w := newFakeWorld(t)
	w.noESO = true
	_, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", ExternalSecrets: "int/jobshout-secrets"}),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "External Secrets Operator not found on cluster k3s1") {
		t.Fatalf("err %v", err)
	}
}

// ── GitHub ─────────────────────────────────────────────────────────────────

func TestGitHub_SealedBoxPutRepoAndEnvironment(t *testing.T) {
	w := newFakeWorld(t)
	gh := NewGitHubClient(w.gh.URL, "gh-token", 0)
	names, err := gh.PutSecrets(context.Background(), "bwalia/jobshout", "", map[string]string{"DB_PASSWORD": "hunter2"}, []string{"DB_PASSWORD"})
	if err != nil || len(names) != 1 {
		t.Fatalf("%v %v", names, err)
	}
	if w.ghPut["/repos/bwalia/jobshout/actions/secrets/DB_PASSWORD"] != "hunter2" {
		t.Fatalf("repo put %#v", w.ghPut)
	}
	if _, err := gh.PutSecrets(context.Background(), "bwalia/jobshout", "prod env", map[string]string{"X": "v2"}, []string{"X"}); err != nil {
		t.Fatal(err)
	}
	if w.ghPut["/repositories/42/environments/prod env/secrets/X"] != "v2" {
		t.Fatalf("env put %#v", w.ghPut)
	}
}

func TestPropagate_SourceGitHubSkipsVault(t *testing.T) {
	w := newFakeWorld(t)
	w.secrets["int/jobshout-review"] = &fakeSecret{rv: 1, data: map[string]string{}}
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Path: "ci/deploy", Keys: []string{"deploy_token"},
		Propagate: mustProp(t, PropagationInput{Source: "github", GitHubRepo: "bwalia/jobshout",
			GitHubNames: "deploy_token=DEPLOY_TOKEN", Cluster: "k3s1", K8sSecrets: "int/jobshout-review"}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, wr := range w.writes {
		if strings.HasPrefix(wr, "POST /v1/") {
			t.Fatalf("vault written with source=github: %v", w.writes)
		}
	}
	v := w.ghPut["/repos/bwalia/jobshout/actions/secrets/DEPLOY_TOKEN"]
	if len(v) < 20 {
		t.Fatalf("github value %q", v)
	}
	if w.secrets["int/jobshout-review"].data["deploy_token"] != b64(v) {
		t.Fatal("k8s secret must get the same generated value")
	}
	if out.DetectedProvider != "github" {
		t.Fatalf("provider %s", out.DetectedProvider)
	}
	assertNoValues(t, out, v)
}

// ── Ring Promoter ──────────────────────────────────────────────────────────

func rpOnly(t *testing.T, w *fakeWorld, rings string) RunOptions {
	return RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "jobshout/int/config", Keys: []string{"password"}, RunID: "run-1",
		Propagate: mustProp(t, PropagationInput{RPRings: rings}),
	}
}

func TestRP_RestartSuccessInOrder(t *testing.T) {
	w := newFakeWorld(t)
	out, err := Execute(context.Background(), w.cfg(), rpOnly(t, w, "int,prod"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.rpBodies) != 2 || w.rpBodies[0]["_ring"] != "int" || w.rpBodies[1]["_ring"] != "prod" {
		t.Fatalf("bodies %#v", w.rpBodies)
	}
	if _, ok := w.rpBodies[0]["password"]; ok {
		t.Fatal("password sent to a non-prod ring")
	}
	if w.rpBodies[1]["password"] != fakeProdPW {
		t.Fatal("prod ring needs the password")
	}
	if !strings.Contains(w.rpBodies[0]["reason"].(string), "secrets rotation run-1 secret/jobshout/int/config") {
		t.Fatalf("reason %v", w.rpBodies[0]["reason"])
	}
	if _, ok := w.rpBodies[0]["deployments"]; ok {
		t.Fatal("without cluster access the RP default set is used")
	}
	var jobs []string
	for _, tg := range out.Result.Propagation.Targets {
		jobs = append(jobs, tg.JobID)
	}
	if strings.Join(jobs, ",") != "job-1,job-2" {
		t.Fatalf("jobs %v", jobs)
	}
}

func TestRP_FailedJobStopsAtFirstRing(t *testing.T) {
	w := newFakeWorld(t)
	w.rpJobs["job-1"] = []string{"running", "failed"}
	out, err := Execute(context.Background(), w.cfg(), rpOnly(t, w, "int,test"), nil)
	if err == nil || !strings.Contains(err.Error(), "rollout timed out") || !strings.Contains(err.Error(), "is at version 5") {
		t.Fatalf("err %v", err)
	}
	if len(w.rpBodies) != 1 {
		t.Fatalf("second ring must not start: %d", len(w.rpBodies))
	}
	tg := out.Result.Propagation.Targets
	if tg[0].Status != "failed" || tg[1].Status != "skipped" {
		t.Fatalf("targets %#v", tg)
	}
}

func TestRP_UnhealthyRingFails(t *testing.T) {
	w := newFakeWorld(t)
	w.rpHealthy["int"] = false
	_, err := Execute(context.Background(), w.cfg(), rpOnly(t, w, "int"), nil)
	if err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf("err %v", err)
	}
}

func TestRP_StartErrorSurfacesMessage(t *testing.T) {
	w := newFakeWorld(t)
	w.rpFailStart = 409
	_, err := Execute(context.Background(), w.cfg(), rpOnly(t, w, "int"), nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 409: nothing to restart") {
		t.Fatalf("err %v", err)
	}
}

func TestRP_ConsumersComputedFromCluster(t *testing.T) {
	w := newFakeWorld(t)
	w.secrets["int/jobshout-cms"] = &fakeSecret{rv: 1, data: map[string]string{}}
	w.secrets["test/jobshout-cms"] = &fakeSecret{rv: 1, data: map[string]string{}}
	w.deploys["int"] = []map[string]any{
		deployment("jobshout-api", map[string]any{"containers": []any{map[string]any{"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "jobshout-cms"}}}}}}),
		deployment("jobshout-web", map[string]any{"containers": []any{map[string]any{"env": []any{map[string]any{"name": "X", "value": "y"}}}}}),
		deployment("worker", map[string]any{"initContainers": []any{map[string]any{"env": []any{map[string]any{"valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "jobshout-cms", "key": "k"}}}}}}}),
		deployment("vol", map[string]any{"volumes": []any{map[string]any{"secret": map[string]any{"secretName": "jobshout-cms"}}}}),
		deployment("proj", map[string]any{"volumes": []any{map[string]any{"projected": map[string]any{"sources": []any{map[string]any{"secret": map[string]any{"name": "jobshout-cms"}}}}}}}),
		deployment("redis", map[string]any{"volumes": []any{map[string]any{"secret": map[string]any{"secretName": "other"}}}}),
	}
	opt := RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", K8sSecrets: "int/jobshout-cms,test/jobshout-cms", RPRings: "int,test,acc"}),
	}
	out, err := Execute(context.Background(), w.cfg(), opt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.rpBodies) != 1 {
		t.Fatalf("only int has consumers; bodies %#v", w.rpBodies)
	}
	got := fmt.Sprint(w.rpBodies[0]["deployments"])
	if got != "[jobshout-api proj vol worker]" {
		t.Fatalf("deployments %s", got)
	}
	var st []string
	for _, tg := range out.Result.Propagation.Targets {
		if tg.Kind == "ring" {
			st = append(st, tg.Name+"="+tg.Status)
		}
	}
	if strings.Join(st, ",") != "jobshout/int=completed,jobshout/test=skipped,jobshout/acc=skipped" {
		t.Fatalf("ring targets %v", st)
	}

	// rp_deployments overrides the computed set.
	w.rpBodies = nil
	opt.Propagate = mustProp(t, PropagationInput{Cluster: "k3s1", K8sSecrets: "int/jobshout-cms", RPRings: "int", RPDeployments: "jobshout-api"})
	if _, err := Execute(context.Background(), w.cfg(), opt, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(w.rpBodies[0]["deployments"]) != "[jobshout-api]" {
		t.Fatalf("override %v", w.rpBodies[0]["deployments"])
	}
}

// ── plan / dry-run / rollback / secrecy ────────────────────────────────────

func allTargets(t *testing.T) Propagation {
	return mustProp(t, PropagationInput{
		Cluster: "k3s1", K8sSecrets: "int/jobshout-cms", ExternalSecrets: "int/jobshout-secrets",
		GitHubRepo: "bwalia/jobshout", RPRings: "int",
	})
}

func seedAll(w *fakeWorld) {
	w.esoReadyAt = 1
	w.secrets["int/jobshout-cms"] = &fakeSecret{rv: 1, data: map[string]string{}}
	w.secrets["int/jobshout-secrets"] = &fakeSecret{rv: 1, data: map[string]string{}}
	w.esos["int/jobshout-secrets"] = &fakeESO{target: "jobshout-secrets"}
	w.deploys["int"] = []map[string]any{
		deployment("jobshout-api", map[string]any{"containers": []any{map[string]any{"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "jobshout-secrets"}}}}}}),
	}
}

func TestPlanAndDryRunPerformZeroWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  RunOptions
	}{
		{"plan", RunOptions{Mode: "plan", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"}}},
		{"dry-rotate", RunOptions{Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"}, DryRun: true, RetireOld: true}},
		{"dry-rollback", RunOptions{Mode: "rollback", Engine: "kv2", Mount: "secret", Path: "p", DryRun: true}},
		{"dry-verify", RunOptions{Mode: "verify", Engine: "kv2", Mount: "secret", Path: "p", DryRun: true}},
		{"github-plan", RunOptions{Mode: "plan", Path: "p", Keys: []string{"k"}}},
		{"github-dry", RunOptions{Mode: "rotate", Path: "p", Keys: []string{"k"}, DryRun: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newFakeWorld(t)
			seedAll(w)
			tc.opt.Propagate = allTargets(t)
			if strings.HasPrefix(tc.name, "github") {
				tc.opt.Propagate = mustProp(t, PropagationInput{Source: "github", GitHubRepo: "bwalia/jobshout", Cluster: "k3s1", K8sSecrets: "int/jobshout-cms", RPRings: "int"})
			}
			out, err := Execute(context.Background(), w.cfg(), tc.opt, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(w.writes) != 0 {
				t.Fatalf("writes: %v", w.writes)
			}
			if tc.opt.Mode == "plan" {
				steps := strings.Join(out.Result.PlanSteps, "\n")
				for _, want := range []string{"int/jobshout-cms", "bwalia/jobshout", "restart jobshout/int"} {
					if !strings.Contains(steps, want) {
						t.Fatalf("plan steps missing %q:\n%s", want, steps)
					}
				}
			} else {
				for _, tg := range out.Result.Propagation.Targets {
					if tg.Status != "dry_run" {
						t.Fatalf("target %#v", tg)
					}
				}
			}
		})
	}
}

func TestRollbackPropagatesRestoredVersion(t *testing.T) {
	w := newFakeWorld(t)
	w.secrets["int/jobshout-cms"] = &fakeSecret{rv: 1, data: map[string]string{}}
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rollback", Engine: "kv2", Mount: "secret", Path: "p", Keys: []string{"password"},
		Propagate: mustProp(t, PropagationInput{Cluster: "k3s1", K8sSecrets: "int/jobshout-cms"}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.secrets["int/jobshout-cms"].data["password"] != b64("old-password-value") {
		t.Fatal("restored value not propagated")
	}
	if out.Result.Propagation.VaultVersion != 5 {
		t.Fatalf("vault version %d", out.Result.Propagation.VaultVersion)
	}
}

func TestNoSecretValueInPhasesOrResult(t *testing.T) {
	w := newFakeWorld(t)
	seedAll(w)
	out, err := Execute(context.Background(), w.cfg(), RunOptions{
		Mode: "rotate", Engine: "kv2", Mount: "secret", Path: "jobshout/int/config", Keys: []string{"password"}, RetireOld: true,
		Propagate: allTargets(t),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	newVal := w.vaultWritten["password"].(string)
	if w.ghPut["/repos/bwalia/jobshout/actions/secrets/PASSWORD"] != newVal {
		t.Fatal("github did not receive the rotated value")
	}
	if fmt.Sprint(w.rpBodies[0]["deployments"]) != "[jobshout-api]" {
		t.Fatalf("deployments %v", w.rpBodies[0]["deployments"])
	}
	assertNoValues(t, out, newVal, "old-password-value")
}

func assertNoValues(t *testing.T, out *RunOutcome, values ...string) {
	t.Helper()
	blob, _ := json.Marshal(struct {
		P any
		R any
	}{out.Phases, out.Result})
	for _, v := range values {
		for _, form := range []string{v, b64(v)} {
			if strings.Contains(string(blob), form) {
				t.Fatalf("secret value leaked into phases/result: %s", blob)
			}
		}
	}
}

func TestScrubRedactsEchoedValues(t *testing.T) {
	pr := &propagator{}
	pr.setValues(map[string]string{"k": "topsecretvalue"})
	if got := pr.scrub("server said topsecretvalue / " + b64("topsecretvalue")); strings.Contains(got, "topsecret") || strings.Contains(got, b64("topsecretvalue")) {
		t.Fatalf("scrub %q", got)
	}
}

func TestParsePropagationValidation(t *testing.T) {
	bad := []PropagationInput{
		{Cluster: "../x"},
		{K8sSecrets: "int/x"}, // no cluster
		{Cluster: "k3s1", K8sSecrets: "justname"},
		{GitHubRepo: "nope"},
		{GitHubNames: "a=B"}, // no repo
		{GitHubRepo: "o/r", GitHubNames: "a=GITHUB_X"},
		{RPRings: "Int"},
		{Source: "github"}, // no repo
		{Source: "github", GitHubRepo: "o/r", Cluster: "k3s1", ExternalSecrets: "int/x"},
		{RPDeployments: "api"}, // no rings
		{Source: "s3"},
	}
	for _, in := range bad {
		if _, err := ParsePropagation(in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	p := mustProp(t, PropagationInput{RPRings: "int, test"})
	if p.RPApp != "jobshout" || p.Source != "vault" {
		t.Fatalf("defaults %+v", p)
	}
	if err := ValidateLaunch("verify", "kv2", nil, mustProp(t, PropagationInput{Source: "github", GitHubRepo: "o/r"})); err == nil {
		t.Fatal("github source must reject verify")
	}
	if err := ValidateLaunch("rotate", "kv2", nil, mustProp(t, PropagationInput{Source: "github", GitHubRepo: "o/r"})); err == nil {
		t.Fatal("github source must require keys")
	}
	if err := ValidateLaunch("rotate", "transit", nil, p); err == nil {
		t.Fatal("transit + targets must be rejected")
	}
}

// Fixture values are assembled at runtime so secret scanners don't flag
// these fakes as hardcoded credentials.
var (
	fakeProdPW = strings.Join([]string{"fixture", "prod"}, "-")
	fakeValue  = strings.Join([]string{"fixture", "value"}, "-")
)
