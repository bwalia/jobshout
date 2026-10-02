// Package appleauth verifies Sign in with Apple identity tokens from the
// native app.
//
// The app asks the server for a one-time nonce, puts SHA-256(nonce) in its
// Apple request, and sends the identity token plus the raw nonce back. The
// server checks the token's signature against Apple's published keys, its
// issuer, audience (our bundle ids), expiry, and that its nonce claim is the
// hash of a nonce this server issued and has not seen used.
package appleauth

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// Issuer is the iss claim on every Apple identity token.
	Issuer = "https://appleid.apple.com"
	// KeysURL publishes Apple's current signing keys.
	KeysURL = "https://appleid.apple.com/auth/keys"
)

// ErrInvalidToken is returned for any token that fails verification. The
// reason is wrapped for logs; callers should not echo it to clients.
var ErrInvalidToken = errors.New("invalid apple identity token")

// Config names the app bundle ids allowed as the token audience. Empty
// disables Sign in with Apple.
type Config struct {
	ClientIDs []string
}

// LoadConfig reads APPLE_CLIENT_IDS (comma-separated bundle ids, e.g.
// "com.jobshout.app,com.jobshout.app.dev").
func LoadConfig() Config {
	var ids []string
	for _, id := range strings.Split(os.Getenv("APPLE_CLIENT_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return Config{ClientIDs: ids}
}

// Configured reports whether any bundle id is allowed.
func (c Config) Configured() bool { return len(c.ClientIDs) > 0 }

// Identity is what a verified token says about the user.
type Identity struct {
	Subject        string
	Email          string
	EmailVerified  bool
	IsPrivateEmail bool
}

// Verifier checks identity tokens.
type Verifier interface {
	Verify(ctx context.Context, identityToken, rawNonce string) (Identity, error)
}

// KeySource returns Apple's public key for a key id.
type KeySource interface {
	Key(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

type verifier struct {
	cfg  Config
	keys KeySource
	now  func() time.Time
}

// NewVerifier builds a Verifier. keys may be nil to fetch from Apple.
func NewVerifier(cfg Config, keys KeySource) Verifier {
	if keys == nil {
		keys = NewJWKS(KeysURL, nil)
	}
	return &verifier{cfg: cfg, keys: keys, now: time.Now}
}

type claims struct {
	Nonce          string `json:"nonce"`
	Email          string `json:"email"`
	EmailVerified  any    `json:"email_verified"`
	IsPrivateEmail any    `json:"is_private_email"`
	jwt.RegisteredClaims
}

// HashNonce is the value the app must put in the Apple request for rawNonce.
func HashNonce(rawNonce string) string {
	sum := sha256.Sum256([]byte(rawNonce))
	return hex.EncodeToString(sum[:])
}

func (v *verifier) Verify(ctx context.Context, identityToken, rawNonce string) (Identity, error) {
	c := &claims{}
	_, err := jwt.ParseWithClaims(identityToken, c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		return v.keys.Key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	aud, _ := c.GetAudience()
	if !slices.ContainsFunc(aud, func(a string) bool { return slices.Contains(v.cfg.ClientIDs, a) }) {
		return Identity{}, fmt.Errorf("%w: audience %v not allowed", ErrInvalidToken, aud)
	}
	if c.Subject == "" {
		return Identity{}, fmt.Errorf("%w: missing sub", ErrInvalidToken)
	}
	if rawNonce == "" || c.Nonce != HashNonce(rawNonce) {
		return Identity{}, fmt.Errorf("%w: nonce mismatch", ErrInvalidToken)
	}

	return Identity{
		Subject:        c.Subject,
		Email:          strings.TrimSpace(c.Email),
		EmailVerified:  truthy(c.EmailVerified),
		IsPrivateEmail: truthy(c.IsPrivateEmail),
	}, nil
}

// Apple has sent these both as JSON booleans and as the strings "true" and
// "false"; accept either.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}

// JWKS fetches and caches Apple's signing keys. An unknown kid triggers a
// refetch (Apple rotates keys), at most once a minute.
type JWKS struct {
	url    string
	client *http.Client

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// NewJWKS builds a key source for url. client may be nil.
func NewJWKS(url string, client *http.Client) *JWKS {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &JWKS{url: url, client: client}
}

const (
	jwksMaxAge      = 24 * time.Hour
	jwksMinRefetch  = time.Minute
	jwksMaxBodySize = 1 << 20
)

func (j *JWKS) Key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if k, ok := j.keys[kid]; ok && time.Since(j.fetchedAt) < jwksMaxAge {
		return k, nil
	}
	if j.keys != nil && time.Since(j.fetchedAt) < jwksMinRefetch {
		if k, ok := j.keys[kid]; ok {
			return k, nil
		}
		return nil, fmt.Errorf("unknown apple key %q", kid)
	}
	keys, err := j.fetch(ctx)
	if err != nil {
		// Serve a stale key rather than fail every sign-in while Apple is down.
		if k, ok := j.keys[kid]; ok {
			return k, nil
		}
		return nil, err
	}
	j.keys, j.fetchedAt = keys, time.Now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown apple key %q", kid)
}

func (j *JWKS) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching apple keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching apple keys: status %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, jwksMaxBodySize)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding apple keys: %w", err)
	}
	out := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaKey(k.N, k.E)
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("apple published no usable keys")
	}
	return out, nil
}

func rsaKey(n, e string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}
	exp := new(big.Int).SetBytes(eb)
	if !exp.IsInt64() || exp.Int64() > 1<<31-1 {
		return nil, errors.New("rsa exponent too large")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(exp.Int64())}, nil
}
