package model

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// The LLM Providers page offers every type the router can run. A type the
// form offers but validation rejects fails only when the user hits Create.
func TestCreateLLMProviderRequestAcceptsEverySupportedType(t *testing.T) {
	v := validator.New()
	for _, typ := range []string{"ollama", "openai", "claude", "gemini"} {
		req := CreateLLMProviderRequest{Name: "My " + typ, ProviderType: typ, DefaultModel: "m"}
		if err := v.Struct(req); err != nil {
			t.Errorf("provider_type %q rejected: %v", typ, err)
		}
	}

	req := CreateLLMProviderRequest{Name: "Bogus", ProviderType: "bogus", DefaultModel: "m"}
	if err := v.Struct(req); err == nil {
		t.Error("provider_type \"bogus\" accepted")
	}
}
