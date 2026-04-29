package llm

// NewTestRouter builds a Router wired to the given clients map with the given
// default provider. It is intended for unit tests that need to inject a stub
// Client without constructing a full config.
//
// This helper is only exposed because the clients map is unexported; kept in
// a non-_test.go file so packages outside llm can use it in their own tests.
func NewTestRouter(defaultProvider string, clients map[string]Client) *Router {
	return &Router{
		clients:         clients,
		defaultProvider: defaultProvider,
	}
}
