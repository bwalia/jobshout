package websocket

import (
	"sync"

	"go.uber.org/zap"
)

// Hub maintains the set of active clients grouped by organisation ID and
// routes broadcast events to the correct org's connections.
//
// All mutations to the clients map are performed from the single Run() goroutine;
// reads from other goroutines use the RWMutex for safe concurrent access.
type Hub struct {
	// clients maps org_id -> set of connected Clients for that org.
	clients map[string]map[*Client]struct{}

	// mu guards reads of the clients map from goroutines outside Run().
	mu sync.RWMutex

	// register queues a client for addition to the hub.
	register chan *Client

	// unregister queues a client for removal from the hub.
	unregister chan *Client

	// broadcast queues events intended for all clients of a given org.
	broadcast chan Event

	logger *zap.Logger
}

// NewHub constructs a Hub with initialised channels and an empty client map.
func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]struct{}),
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
		broadcast:  make(chan Event, 256),
		logger:     logger,
	}
}

// Run starts the Hub's main event loop.  It must be called in its own goroutine.
// Stopping the loop is achieved by cancelling the context that owns the server
// lifecycle; when the HTTP server shuts down, all client connections are closed,
// which drains the unregister channel naturally.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.addClient(client)

		case client := <-h.unregister:
			h.removeClient(client)

		case event := <-h.broadcast:
			h.dispatchEvent(event)
		}
	}
}

// BroadcastToOrg enqueues an event for all clients belonging to the given org.
// It is safe to call from any goroutine.
func (h *Hub) BroadcastToOrg(orgID string, event Event) {
	// Stamp the org onto the event so recipients can verify the scope.
	event.OrgID = orgID
	h.broadcast <- event
}

// Register enqueues a new client for the hub.
// Call this immediately after creating a Client, before starting its pumps.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// ConnectionCount returns the number of live connections across all orgs.
// Useful for health-check or metrics endpoints.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	total := 0
	for _, orgClients := range h.clients {
		total += len(orgClients)
	}
	return total
}

// OrgConnectionCount returns the number of live connections for a specific org.
func (h *Hub) OrgConnectionCount(orgID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients[orgID])
}

// ---------------------------------------------------------------------------
// Internal helpers - only called from Run() to avoid data races
// ---------------------------------------------------------------------------

func (h *Hub) addClient(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client.orgID]; !ok {
		h.clients[client.orgID] = make(map[*Client]struct{})
	}
	h.clients[client.orgID][client] = struct{}{}
	h.mu.Unlock()

	h.logger.Info("ws client registered",
		zap.String("org_id", client.orgID),
		zap.String("user_id", client.userID),
	)
}

func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	if orgClients, ok := h.clients[client.orgID]; ok {
		if _, exists := orgClients[client]; exists {
			delete(orgClients, client)
			// Avoid holding empty nested maps in memory indefinitely.
			if len(orgClients) == 0 {
				delete(h.clients, client.orgID)
			}
		}
	}
	h.mu.Unlock()

	h.logger.Info("ws client unregistered",
		zap.String("org_id", client.orgID),
		zap.String("user_id", client.userID),
	)
}

func (h *Hub) dispatchEvent(event Event) {
	h.mu.RLock()
	orgClients, ok := h.clients[event.OrgID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Copy the set under the read lock so we can iterate without holding it.
	targets := make([]*Client, 0, len(orgClients))
	for client := range orgClients {
		targets = append(targets, client)
	}
	h.mu.RUnlock()

	for _, client := range targets {
		client.SendEvent(event)
	}
}
