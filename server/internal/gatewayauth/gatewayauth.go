// Package gatewayauth signs outbound requests to the model services JobShout
// runs on the workstation.
//
// Those services — Ollama for language models, the image service for pictures —
// sit outside the cluster behind a JWT-verifying gateway, because MLX and large
// model weights cannot be scheduled onto amd64 k3s nodes. Every ring reaches
// them over the public internet, so every request has to prove which app it came
// from.
//
// The scheme lives here rather than in one of the callers so that there is
// exactly one answer to "how does JobShout authenticate to a workstation model
// service". A second implementation would be a second thing to rotate and a
// second thing to get subtly wrong.
package gatewayauth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AppName identifies this application in the JWT presented to a gateway. It is
// the value of the "app" claim.
const AppName = "jobshout"

// TokenTTL is how long a minted token stays valid. Short on purpose: a token is
// used for exactly one request, so it never needs to outlive that request by
// much.
const TokenTTL = 10 * time.Minute

// Signer signs requests to a service behind a JWT-verifying gateway.
//
// A nil *Signer (or one with no secret) is valid and means "talk to the service
// directly" — a plain local Ollama or a locally-run image service has no gateway
// and rejects nothing, so the same client works for both without a second code
// path.
type Signer struct {
	secret []byte
}

// New returns nil when no secret is configured, so callers can hold a
// possibly-nil *Signer and call Apply on it unconditionally.
func New(secret string) *Signer {
	if secret == "" {
		return nil
	}
	return &Signer{secret: []byte(secret)}
}

// Enabled reports whether requests will be signed.
func (s *Signer) Enabled() bool { return s != nil && len(s.secret) > 0 }

// Apply mints a fresh token and attaches it to req.
//
// A new token is minted for every single request and never cached: the tokens
// are cheap to produce, and reusing one across requests is what turns a single
// clock skew or rotation into a stream of failures rather than one.
func (s *Signer) Apply(req *http.Request) error {
	if !s.Enabled() {
		return nil
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"app": AppName,
		"iat": now.Unix(),
		"exp": now.Add(TokenTTL).Unix(),
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return fmt.Errorf("gatewayauth: sign token: %w", err)
	}

	// The gateway expects the bare token — no "Bearer " prefix.
	req.Header.Set("x-api-key", signed)
	return nil
}

// IsAuthStatus reports whether a status code is a gateway auth rejection.
func IsAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}
