package database

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
)

// closedPortDSN returns a DSN pointing at a loopback port with nothing behind
// it, so connections are refused immediately rather than hanging.
func closedPortDSN(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}

	return fmt.Sprintf("postgres://u:p@127.0.0.1:%d/db?sslmode=disable&connect_timeout=1", port)
}

func TestNewPoolWithRetry_RetriesUntilTimeout(t *testing.T) {
	dsn := closedPortDSN(t)

	start := time.Now()
	pool, err := NewPoolWithRetry(context.Background(), dsn, zap.NewNop(), 2*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("expected an error connecting to a closed port, got nil")
	}

	// The first backoff is 1s, so anything faster means we never retried at
	// all — which is the regression this whole function exists to prevent.
	if elapsed < time.Second {
		t.Errorf("returned after %s; expected at least one 1s backoff", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("returned after %s; expected to give up near the 2s timeout", elapsed)
	}
}

func TestNewPoolWithRetry_ZeroTimeoutFailsFast(t *testing.T) {
	dsn := closedPortDSN(t)

	start := time.Now()
	pool, err := NewPoolWithRetry(context.Background(), dsn, zap.NewNop(), 0)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("expected an error connecting to a closed port, got nil")
	}
	if elapsed >= time.Second {
		t.Errorf("returned after %s; a zero timeout must not back off at all", elapsed)
	}
}

func TestNewPoolWithRetry_HonoursContextCancellation(t *testing.T) {
	dsn := closedPortDSN(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	start := time.Now()
	pool, err := NewPoolWithRetry(ctx, dsn, zap.NewNop(), time.Minute)
	elapsed := time.Since(start)

	if err == nil {
		pool.Close()
		t.Fatal("expected an error after cancellation, got nil")
	}

	// A cancelled context must abort the wait rather than sit out the full
	// minute, so shutdown is not blocked behind a doomed retry loop.
	if elapsed > 3*time.Second {
		t.Errorf("returned after %s; expected cancellation to cut the backoff short", elapsed)
	}
}
