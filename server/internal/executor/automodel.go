package executor

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/modelselect"
)

// modelChoice is the outcome of deciding which model runs one call.
type modelChoice struct {
	Client   llm.Client
	Provider string
	Model    string
	// Reason is set only when auto-selection ran, and explains the pick.
	Reason string
}

// resolveModel decides which provider and model handle a single call.
//
// Auto is opt-in and deliberately narrow: it engages only when the agent's
// ModelProvider is literally "auto" AND a selector has been wired in. Every
// other configuration — an explicit provider, an explicit provider plus model,
// or nothing at all — resolves exactly as it did before this function existed.
func resolveModel(
	router *llm.Router,
	sel *modelselect.Selector,
	log *zap.Logger,
	agent *model.Agent,
	sig modelselect.TaskSignals,
) (modelChoice, error) {
	providerName := ""
	if agent.ModelProvider != nil {
		providerName = *agent.ModelProvider
	}
	modelName := ""
	if agent.ModelName != nil {
		modelName = *agent.ModelName
	}

	if modelselect.IsAuto(providerName) {
		if sel == nil {
			// Auto was asked for but is not available in this build/config.
			// Fall through to the configured default rather than failing: an
			// agent set to "auto" must never be worse off than one that never
			// asked, and "auto" is not a real provider name the Router knows.
			providerName = ""
		} else {
			dec, err := sel.Select(sig, modelselect.Constraints{})
			if err != nil {
				return modelChoice{}, fmt.Errorf("executor: auto model selection: %w", err)
			}
			client, err := router.For(dec.Provider)
			if err != nil {
				return modelChoice{}, fmt.Errorf("executor: auto-selected provider %q: %w", dec.Provider, err)
			}
			if log != nil {
				log.Info("auto-selected model",
					zap.String("provider", dec.Provider),
					zap.String("model", dec.Model),
					zap.String("kind", sig.Kind),
					zap.String("reason", dec.Reason),
				)
			}
			return modelChoice{
				Client:   client,
				Provider: dec.Provider,
				Model:    dec.Model,
				Reason:   dec.Reason,
			}, nil
		}
	}

	client, err := router.For(providerName)
	if err != nil {
		return modelChoice{}, fmt.Errorf("executor: resolve LLM client: %w", err)
	}
	return modelChoice{
		Client:   client,
		Provider: client.ProviderName(),
		Model:    modelName,
	}, nil
}

// estimateTokens approximates a prompt's token count at roughly four characters
// per token. It only feeds the context-window check, where being in the right
// ballpark is enough — running a real tokenizer on every call would cost more
// than the mistake it prevents.
func estimateTokens(parts ...string) int {
	chars := 0
	for _, p := range parts {
		chars += len(p)
	}
	return chars / 4
}
