package mail

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := KeyFromSecret("unit-test-token-key")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := Encrypt(key, []byte("refresh-token-value"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ct), "refresh-token-value") {
		t.Fatal("ciphertext contained the plaintext")
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "refresh-token-value" {
		t.Errorf("got %q", pt)
	}
}

func TestKeyFromSecretHex(t *testing.T) {
	hexKey := strings.Repeat("ab", 32)
	k, err := KeyFromSecret(hexKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != 32 {
		t.Fatalf("len %d", len(k))
	}
}

func TestRedactStripsTokens(t *testing.T) {
	in := "token exchange failed: refresh_token=ya29.secret&access_token=abc code=4/xyz"
	out := Redact(in)
	if strings.Contains(out, "ya29") || strings.Contains(out, "abc") || strings.Contains(out, "4/xyz") {
		t.Errorf("leaked: %s", out)
	}
	if !strings.Contains(out, "refresh_token=<redacted>") {
		t.Errorf("expected redaction marker: %s", out)
	}
}
