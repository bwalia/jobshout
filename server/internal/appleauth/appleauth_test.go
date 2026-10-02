package appleauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type keyServer struct {
	srv   *httptest.Server
	keys  map[string]*rsa.PrivateKey
	hits  atomic.Int32
	extra atomic.Bool // publish "k2" too
}

func newKeyServer(t *testing.T) *keyServer {
	t.Helper()
	ks := &keyServer{keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range []string{"k1", "k2"} {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		ks.keys[kid] = k
	}
	ks.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ks.hits.Add(1)
		kids := []string{"k1"}
		if ks.extra.Load() {
			kids = append(kids, "k2")
		}
		var out []map[string]string
		for _, kid := range kids {
			pub := ks.keys[kid].PublicKey
			out = append(out, map[string]string{
				"kid": kid, "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": out})
	}))
	t.Cleanup(ks.srv.Close)
	return ks
}

func (ks *keyServer) sign(t *testing.T, kid string, c jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(ks.keys[kid])
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validClaims(nonce string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":            Issuer,
		"aud":            "com.jobshout.app",
		"sub":            "001234.abcd",
		"iat":            now.Unix(),
		"exp":            now.Add(5 * time.Minute).Unix(),
		"nonce":          HashNonce(nonce),
		"email":          "ada@example.com",
		"email_verified": "true",
	}
}

func TestVerify(t *testing.T) {
	ks := newKeyServer(t)
	v := NewVerifier(Config{ClientIDs: []string{"com.jobshout.app"}}, NewJWKS(ks.srv.URL, nil))
	ctx := context.Background()
	const nonce = "raw-nonce-value-0123456789"

	id, err := v.Verify(ctx, ks.sign(t, "k1", validClaims(nonce)), nonce)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if id.Subject != "001234.abcd" || id.Email != "ada@example.com" || !id.EmailVerified {
		t.Fatalf("unexpected identity: %+v", id)
	}

	cases := map[string]func(jwt.MapClaims){
		"wrong audience": func(c jwt.MapClaims) { c["aud"] = "com.someone.else" },
		"wrong issuer":   func(c jwt.MapClaims) { c["iss"] = "https://evil.example" },
		"expired":        func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"wrong nonce":    func(c jwt.MapClaims) { c["nonce"] = HashNonce("other") },
		"raw nonce":      func(c jwt.MapClaims) { c["nonce"] = nonce },
		"no subject":     func(c jwt.MapClaims) { delete(c, "sub") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validClaims(nonce)
			mutate(c)
			if _, err := v.Verify(ctx, ks.sign(t, "k1", c), nonce); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("got %v, want ErrInvalidToken", err)
			}
		})
	}

	t.Run("hs256 with public key as secret", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims(nonce))
		tok.Header["kid"] = "k1"
		s, _ := tok.SignedString([]byte("anything"))
		if _, err := v.Verify(ctx, s, nonce); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestJWKSRefetchesOnUnknownKidAtMostOncePerMinute(t *testing.T) {
	ks := newKeyServer(t)
	j := NewJWKS(ks.srv.URL, nil)
	ctx := context.Background()

	if _, err := j.Key(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	// Apple rotates in k2, but we fetched moments ago: no refetch yet.
	ks.extra.Store(true)
	if _, err := j.Key(ctx, "k2"); err == nil {
		t.Fatal("expected unknown kid inside the refetch window")
	}
	if ks.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", ks.hits.Load())
	}

	j.mu.Lock()
	j.fetchedAt = time.Now().Add(-2 * jwksMinRefetch)
	j.mu.Unlock()
	if _, err := j.Key(ctx, "k2"); err != nil {
		t.Fatalf("rotated key not picked up: %v", err)
	}
	if ks.hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", ks.hits.Load())
	}
}
