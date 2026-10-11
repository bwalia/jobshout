package secretbox

import (
	"errors"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	b, err := New("a passphrase, not hex")
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Join([]string{"sk", "test", "123"}, "-")
	sealed, err := b.Seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(sealed) || strings.Contains(sealed, secret) {
		t.Fatalf("sealed value leaks plaintext: %q", sealed)
	}
	again, _ := b.Seal(secret)
	if again == sealed {
		t.Fatal("two seals of one value are identical (nonce reuse)")
	}
	got, err := b.Open(sealed)
	if err != nil || got != secret {
		t.Fatalf("open = %q, %v", got, err)
	}
	// Sealing a sealed value is a no-op, not double encryption.
	if s2, _ := b.Seal(sealed); s2 != sealed {
		t.Fatal("re-seal changed a sealed value")
	}
}

func TestLegacyPlaintextAndNilBox(t *testing.T) {
	var nilBox *Box
	if got, _ := nilBox.Seal("plain"); got != "plain" {
		t.Fatalf("nil box seal = %q", got)
	}
	b, _ := New(strings.Repeat("ab", 32)) // 64 hex chars = raw key
	if got, err := b.Open("legacy-plain"); err != nil || got != "legacy-plain" {
		t.Fatalf("legacy open = %q, %v", got, err)
	}
	sealed, _ := b.Seal("x")
	if _, err := nilBox.Open(sealed); !errors.Is(err, ErrNoKey) {
		t.Fatalf("nil box open sealed: err = %v, want ErrNoKey", err)
	}
	other, _ := New("different")
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("wrong key opened a sealed value")
	}
	if empty, _ := New("  "); empty != nil {
		t.Fatal("blank secret should be the plaintext (nil) box")
	}
}
