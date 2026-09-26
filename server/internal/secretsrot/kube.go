package secretsrot

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// A deliberately small Kubernetes REST client. It only needs to GET/PATCH
// Secrets and ExternalSecrets, so pulling in client-go (tens of MB of
// dependencies) is not worth it. Supported kubeconfig auth: client cert/key
// (inline or file), bearer token (inline or tokenFile), CA (inline or file),
// insecure-skip-tls-verify, tls-server-name, current-context. exec and
// auth-provider plugins are rejected with a clear error.

var clusterNameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidClusterName reports whether name is a safe kubeconfig name (no paths).
func ValidClusterName(name string) bool {
	return len(name) <= 63 && clusterNameRe.MatchString(name)
}

type kubeconfigFile struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthority     string `yaml:"certificate-authority"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
			TLSServerName            string `yaml:"tls-server-name"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificate     string         `yaml:"client-certificate"`
			ClientCertificateData string         `yaml:"client-certificate-data"`
			ClientKey             string         `yaml:"client-key"`
			ClientKeyData         string         `yaml:"client-key-data"`
			Token                 string         `yaml:"token"`
			TokenFile             string         `yaml:"tokenFile"`
			Exec                  map[string]any `yaml:"exec"`
			AuthProvider          map[string]any `yaml:"auth-provider"`
		} `yaml:"user"`
	} `yaml:"users"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

// KubeClient is a minimal authenticated client for one API server.
type KubeClient struct {
	server string
	token  string
	http   *http.Client
}

// kubeconfigEnvName maps a cluster name to SECRETS_ROT_KUBECONFIG_<UPPER>.
func kubeconfigEnvName(cluster string) string {
	return "SECRETS_ROT_KUBECONFIG_" + strings.ToUpper(strings.ReplaceAll(cluster, "-", "_"))
}

// LoadKubeconfigBytes returns the named cluster's kubeconfig content from
// SECRETS_ROT_KUBECONFIG_<NAME>, else <dir>/<name>, else <dir>/<name>.yaml.
func LoadKubeconfigBytes(dir, cluster string) ([]byte, error) {
	if !ValidClusterName(cluster) {
		return nil, fmt.Errorf("invalid cluster name %q (allowed: a-z 0-9 -)", cluster)
	}
	if v := os.Getenv(kubeconfigEnvName(cluster)); strings.TrimSpace(v) != "" {
		return []byte(v), nil
	}
	if dir == "" {
		dir = DefaultKubeconfigDir
	}
	for _, name := range []string{cluster, cluster + ".yaml"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			return b, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read kubeconfig for %s: %w", cluster, err)
		}
	}
	return nil, fmt.Errorf("no kubeconfig for cluster %q: set %s or mount %s/%s", cluster, kubeconfigEnvName(cluster), dir, cluster)
}

// NewKubeClientForCluster loads the named kubeconfig and builds a client.
func NewKubeClientForCluster(dir, cluster string, timeout time.Duration) (*KubeClient, error) {
	raw, err := LoadKubeconfigBytes(dir, cluster)
	if err != nil {
		return nil, err
	}
	kc, err := ParseKubeconfig(raw, timeout)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig %s: %w", cluster, err)
	}
	return kc, nil
}

// ParseKubeconfig builds a client from kubeconfig YAML using current-context
// (or the only context when current-context is empty).
func ParseKubeconfig(raw []byte, timeout time.Duration) (*KubeClient, error) {
	var f kubeconfigFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	ctxName := f.CurrentContext
	if ctxName == "" {
		if len(f.Contexts) != 1 {
			return nil, fmt.Errorf("current-context is empty and there are %d contexts", len(f.Contexts))
		}
		ctxName = f.Contexts[0].Name
	}
	var clusterName, userName string
	found := false
	for _, c := range f.Contexts {
		if c.Name == ctxName {
			clusterName, userName, found = c.Context.Cluster, c.Context.User, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("context %q not found", ctxName)
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	server := ""
	found = false
	for _, c := range f.Clusters {
		if c.Name != clusterName {
			continue
		}
		found = true
		server = strings.TrimRight(c.Cluster.Server, "/")
		tlsCfg.ServerName = c.Cluster.TLSServerName
		if c.Cluster.InsecureSkipTLSVerify {
			tlsCfg.InsecureSkipVerify = true //nolint:gosec // explicit kubeconfig opt-in
		}
		caPEM, err := dataOrFile(c.Cluster.CertificateAuthorityData, c.Cluster.CertificateAuthority)
		if err != nil {
			return nil, fmt.Errorf("certificate-authority: %w", err)
		}
		if len(caPEM) > 0 {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return nil, fmt.Errorf("certificate-authority-data holds no PEM certificates")
			}
			tlsCfg.RootCAs = pool
		}
	}
	if !found {
		return nil, fmt.Errorf("cluster %q not found", clusterName)
	}
	if server == "" {
		return nil, fmt.Errorf("cluster %q has no server", clusterName)
	}

	token := ""
	found = false
	for _, u := range f.Users {
		if u.Name != userName {
			continue
		}
		found = true
		if u.User.Exec != nil {
			return nil, fmt.Errorf("user %q uses an exec credential plugin, which is not supported; use a client certificate or a ServiceAccount token", userName)
		}
		if u.User.AuthProvider != nil {
			return nil, fmt.Errorf("user %q uses an auth-provider plugin, which is not supported; use a client certificate or a ServiceAccount token", userName)
		}
		certPEM, err := dataOrFile(u.User.ClientCertificateData, u.User.ClientCertificate)
		if err != nil {
			return nil, fmt.Errorf("client-certificate: %w", err)
		}
		keyPEM, err := dataOrFile(u.User.ClientKeyData, u.User.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("client-key: %w", err)
		}
		if len(certPEM) > 0 || len(keyPEM) > 0 {
			pair, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("client certificate/key: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{pair}
		}
		token = strings.TrimSpace(u.User.Token)
		if token == "" && u.User.TokenFile != "" {
			b, err := os.ReadFile(u.User.TokenFile)
			if err != nil {
				return nil, fmt.Errorf("tokenFile: %w", err)
			}
			token = strings.TrimSpace(string(b))
		}
	}
	if !found && userName != "" {
		return nil, fmt.Errorf("user %q not found", userName)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	return &KubeClient{server: server, token: token, http: &http.Client{Timeout: timeout, Transport: tr}}, nil
}

func dataOrFile(data, file string) ([]byte, error) {
	if strings.TrimSpace(data) != "" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(data))
		if err != nil {
			return nil, fmt.Errorf("invalid base64")
		}
		return b, nil
	}
	if file != "" {
		return os.ReadFile(file)
	}
	return nil, nil
}

// Server is the API server URL (not sensitive).
func (k *KubeClient) Server() string { return k.server }

// errKubeNotFound marks a 404 so callers can try another API version.
var errKubeNotFound = errors.New("not found")

func (k *KubeClient) do(ctx context.Context, method, path, contentType string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.server+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if k.token != "" {
		req.Header.Set("Authorization", "Bearer "+k.token)
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusNotFound {
		return errKubeNotFound
	}
	if resp.StatusCode >= 300 {
		// Kubernetes Status bodies carry a message, never Secret data.
		var st struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &st)
		msg := st.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		return fmt.Errorf("kubernetes HTTP %d: %s", resp.StatusCode, msg)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
	}
	return nil
}

// objectMeta is the subset of metadata the agent reads.
type objectMeta struct {
	Metadata struct {
		ResourceVersion string            `json:"resourceVersion"`
		Labels          map[string]string `json:"labels"`
		Annotations     map[string]string `json:"annotations"`
	} `json:"metadata"`
}

// SecretInfo is a Secret's metadata (never its data).
type SecretInfo struct {
	Exists          bool
	ResourceVersion string
	HelmManaged     bool
}

// GetSecretInfo reads a Secret's metadata. Only metadata is decoded; the data
// field is discarded unread.
func (k *KubeClient) GetSecretInfo(ctx context.Context, ns, name string) (SecretInfo, error) {
	var m objectMeta
	err := k.do(ctx, http.MethodGet, secretPath(ns, name), "", nil, &m)
	if errors.Is(err, errKubeNotFound) {
		return SecretInfo{}, nil
	}
	if err != nil {
		return SecretInfo{}, err
	}
	return SecretInfo{
		Exists:          true,
		ResourceVersion: m.Metadata.ResourceVersion,
		HelmManaged:     m.Metadata.Labels["app.kubernetes.io/managed-by"] == "Helm" || m.Metadata.Annotations["meta.helm.sh/release-name"] != "",
	}, nil
}

func secretPath(ns, name string) string {
	return "/api/v1/namespaces/" + url.PathEscape(ns) + "/secrets/" + url.PathEscape(name)
}

// SecretResourceVersion returns the Secret's resourceVersion ("" if missing).
// Only metadata is decoded; data is discarded unread.
func (k *KubeClient) SecretResourceVersion(ctx context.Context, ns, name string) (string, error) {
	var m objectMeta
	err := k.do(ctx, http.MethodGet, secretPath(ns, name), "", nil, &m)
	if errors.Is(err, errKubeNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return m.Metadata.ResourceVersion, nil
}

// SecretPatchBody is the JSON merge patch that sets only the given keys.
// Other keys in .data are left untouched by the API server.
func SecretPatchBody(values map[string]string) map[string]any {
	data := make(map[string]string, len(values))
	for k, v := range values {
		data[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}
	return map[string]any{"data": data}
}

// PatchSecretKeys merges the rotated keys into an existing Secret and returns
// the new resourceVersion.
func (k *KubeClient) PatchSecretKeys(ctx context.Context, ns, name string, values map[string]string) (string, error) {
	var m objectMeta
	err := k.do(ctx, http.MethodPatch, secretPath(ns, name), "application/merge-patch+json", SecretPatchBody(values), &m)
	if errors.Is(err, errKubeNotFound) {
		return "", fmt.Errorf("secret %s/%s not found", ns, name)
	}
	if err != nil {
		return "", err
	}
	return m.Metadata.ResourceVersion, nil
}

// externalSecret is the subset of an ExternalSecret the agent reads.
type externalSecret struct {
	Metadata struct {
		Name            string `json:"name"`
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Spec struct {
		Target struct {
			Name string `json:"name"`
		} `json:"target"`
	} `json:"spec"`
	Status struct {
		RefreshTime string `json:"refreshTime"`
		Conditions  []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

func (e externalSecret) targetName() string {
	if e.Spec.Target.Name != "" {
		return e.Spec.Target.Name
	}
	return e.Metadata.Name
}

// esoVersions are tried in order; the chart ships v1beta1, newer ESO serves v1.
var esoVersions = []string{"v1beta1", "v1"}

func esPath(version, ns, name string) string {
	return "/apis/external-secrets.io/" + version + "/namespaces/" + url.PathEscape(ns) + "/externalsecrets/" + url.PathEscape(name)
}

// errNoESO means the external-secrets.io API group is not served.
var errNoESO = errors.New("External Secrets Operator not found")

// GetExternalSecret fetches an ExternalSecret and returns the API version that served it.
func (k *KubeClient) GetExternalSecret(ctx context.Context, ns, name string) (externalSecret, string, error) {
	for _, v := range esoVersions {
		var es externalSecret
		err := k.do(ctx, http.MethodGet, esPath(v, ns, name), "", nil, &es)
		if errors.Is(err, errKubeNotFound) {
			continue
		}
		if err != nil {
			return es, "", err
		}
		return es, v, nil
	}
	// Distinguish "operator not installed" from "object missing".
	if err := k.do(ctx, http.MethodGet, "/apis/external-secrets.io", "", nil, nil); errors.Is(err, errKubeNotFound) {
		return externalSecret{}, "", errNoESO
	}
	return externalSecret{}, "", fmt.Errorf("externalsecret %s/%s not found", ns, name)
}

// podSpecRefs is the subset of a pod template that can reference Secrets.
type podSpecRefs struct {
	Containers     []containerRefs `json:"containers"`
	InitContainers []containerRefs `json:"initContainers"`
	Volumes        []struct {
		Secret *struct {
			SecretName string `json:"secretName"`
		} `json:"secret"`
		Projected *struct {
			Sources []struct {
				Secret *struct {
					Name string `json:"name"`
				} `json:"secret"`
			} `json:"sources"`
		} `json:"projected"`
	} `json:"volumes"`
}

type containerRefs struct {
	EnvFrom []struct {
		SecretRef *struct {
			Name string `json:"name"`
		} `json:"secretRef"`
	} `json:"envFrom"`
	Env []struct {
		ValueFrom *struct {
			SecretKeyRef *struct {
				Name string `json:"name"`
			} `json:"secretKeyRef"`
		} `json:"valueFrom"`
	} `json:"env"`
}

// secretNames returns every Secret name a pod spec references.
func (p podSpecRefs) secretNames() map[string]bool {
	out := map[string]bool{}
	for _, c := range append(append([]containerRefs{}, p.Containers...), p.InitContainers...) {
		for _, ef := range c.EnvFrom {
			if ef.SecretRef != nil && ef.SecretRef.Name != "" {
				out[ef.SecretRef.Name] = true
			}
		}
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil && e.ValueFrom.SecretKeyRef.Name != "" {
				out[e.ValueFrom.SecretKeyRef.Name] = true
			}
		}
	}
	for _, v := range p.Volumes {
		if v.Secret != nil && v.Secret.SecretName != "" {
			out[v.Secret.SecretName] = true
		}
		if v.Projected != nil {
			for _, s := range v.Projected.Sources {
				if s.Secret != nil && s.Secret.Name != "" {
					out[s.Secret.Name] = true
				}
			}
		}
	}
	return out
}

// SecretConsumers lists the Deployments in ns whose pod template references
// any of secrets (envFrom, secretKeyRef, secret / projected volumes).
func (k *KubeClient) SecretConsumers(ctx context.Context, ns string, secrets map[string]bool) ([]string, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Template struct {
					Spec podSpecRefs `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		} `json:"items"`
	}
	err := k.do(ctx, http.MethodGet, "/apis/apps/v1/namespaces/"+url.PathEscape(ns)+"/deployments", "", nil, &list)
	if errors.Is(err, errKubeNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list deployments in %s: %w", ns, err)
	}
	var out []string
	for _, d := range list.Items {
		refs := d.Spec.Template.Spec.secretNames()
		for s := range secrets {
			if refs[s] {
				out = append(out, d.Metadata.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// AnnotateForceSync sets force-sync=<ts> on an ExternalSecret (merge patch),
// which makes the External Secrets Operator re-read the store immediately.
func (k *KubeClient) AnnotateForceSync(ctx context.Context, version, ns, name string, ts int64) error {
	body := map[string]any{"metadata": map[string]any{"annotations": map[string]string{"force-sync": fmt.Sprintf("%d", ts)}}}
	err := k.do(ctx, http.MethodPatch, esPath(version, ns, name), "application/merge-patch+json", body, nil)
	if errors.Is(err, errKubeNotFound) {
		return fmt.Errorf("externalsecret %s/%s not found", ns, name)
	}
	return err
}
