package companyweb

import (
	"context"

	"github.com/jobshout/server/internal/research"
)

// ClientRunner adapts research.Client to Runner.
type ClientRunner struct {
	Client *research.Client
}

// Available is true when general web search (Brave) is among the backends.
func (r ClientRunner) Available() bool {
	if r.Client == nil {
		return false
	}
	for _, name := range r.Client.SearchBackends() {
		if name == "web" {
			return true
		}
	}
	return false
}

// Lookup runs web search and ranks official-site candidates.
func (r ClientRunner) Lookup(ctx context.Context, companyName string) ([]Result, error) {
	return Lookup(ctx, clientSearcher{r.Client}, companyName)
}

type clientSearcher struct{ c *research.Client }

func (s clientSearcher) Available() bool {
	return ClientRunner{Client: s.c}.Available()
}

func (s clientSearcher) Search(ctx context.Context, query string, limit int) ([]research.Source, error) {
	return s.c.Search(ctx, query, limit)
}
