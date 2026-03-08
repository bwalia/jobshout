package websocket

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Maximum time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Maximum time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Interval between ping messages sent to the peer (must be less than pongWait).
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from the peer (bytes).
	maxMessageSize = 8 * 1024

	// Buffer size for the per-client outbound message channel.
	sendChannelBuffer = 256
)

// Upgrader configures the HTTP-to-WebSocket upgrade.
// CheckOrigin is left permissive here; restrict AllowedOrigins in production
// via the CORS configuration applied at the router level.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client represents a single connected WebSocket peer.
// Each client is associated with exactly one organisation and one user.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	orgID  string
	userID string

	// send is a buffered channel of outbound JSON-encoded events.
	// The writePump drains this channel and forwards messages to the peer.
	send chan []byte

	logger *zap.Logger
}

// NewClient constructs a Client and upgrades the HTTP connection to WebSocket.
// Returns an error if the upgrade fails.
func NewClient(
	hub *Hub,
	w http.ResponseWriter,
	r *http.Request,
	orgID string,
	userID string,
	logger *zap.Logger,
) (*Client, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}

	return &Client{
		hub:    hub,
		conn:   conn,
		orgID:  orgID,
		userID: userID,
		send:   make(chan []byte, sendChannelBuffer),
		logger: logger,
	}, nil
}

// readPump pumps inbound messages from the WebSocket connection into the hub.
// It runs in its own goroutine for each connected client.
//
// The application ensures that there is at most one reader on a connection by
// executing all reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		// Deregister on disconnect so the hub stops routing messages to this client.
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)

	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.logger.Warn("failed to set read deadline", zap.Error(err))
	}

	// Reset read deadline on every pong to keep the connection alive.
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				c.logger.Warn("unexpected websocket close",
					zap.String("org_id", c.orgID),
					zap.String("user_id", c.userID),
					zap.Error(err),
				)
			}
			break
		}

		// Currently clients are read-only consumers; incoming messages are logged
		// for observability and discarded. Extend here to handle client-initiated
		// events if needed.
		c.logger.Debug("received ws message",
			zap.String("org_id", c.orgID),
			zap.String("user_id", c.userID),
			zap.ByteString("message", message),
		)
	}
}

// writePump pumps outbound messages from the send channel to the WebSocket connection.
// It runs in its own goroutine for each connected client.
//
// A single writePump goroutine is used per connection to ensure that all writes
// are serialised (the gorilla/websocket library requires this).
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("failed to set write deadline", zap.Error(err))
				return
			}

			if !ok {
				// The hub closed the send channel; send a close frame and exit.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				c.logger.Warn("failed to write ws message", zap.Error(err))
			}

			// Flush any additional queued messages in the same write frame to
			// reduce the number of system calls.
			pending := len(c.send)
			for i := 0; i < pending; i++ {
				if _, err := w.Write([]byte{'\n'}); err != nil {
					c.logger.Warn("failed to write newline separator", zap.Error(err))
				}
				if _, err := w.Write(<-c.send); err != nil {
					c.logger.Warn("failed to write batched ws message", zap.Error(err))
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			// Send a ping to detect dead connections.
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Warn("failed to set write deadline for ping", zap.Error(err))
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Start launches the read and write pumps in separate goroutines.
// Call this after registering the client with the hub.
func (c *Client) Start() {
	go c.writePump()
	go c.readPump()
}

// SendEvent serialises the event to JSON and queues it on the client's send channel.
// It is safe to call from any goroutine.
func (c *Client) SendEvent(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		c.logger.Error("failed to marshal event",
			zap.String("event_type", event.Type),
			zap.Error(err),
		)
		return
	}

	select {
	case c.send <- data:
	default:
		// The send buffer is full; the client is likely a slow consumer.
		// Deregister it to prevent the hub from blocking indefinitely.
		c.hub.unregister <- c
		close(c.send)
	}
}
