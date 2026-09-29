package opsapi

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsUpdateForbidden(t *testing.T) {
	err := fmt.Errorf("%w: detail", ErrUpdateForbidden)
	if !IsUpdateForbidden(err) {
		t.Fatal("wrapped ErrUpdateForbidden should match")
	}
	if IsUpdateForbidden(errors.New("other")) {
		t.Fatal("unrelated error must not match")
	}
}
