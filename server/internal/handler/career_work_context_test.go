package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// A person navigating away cancels the request. Work already under way must
// keep going so what it saves is there when they come back.
func TestWorkContext_SurvivesTheRequestBeingCancelled(t *testing.T) {
	reqCtx, cancelRequest := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "/api/v1/career/pipeline/batch", nil).WithContext(reqCtx)

	ctx, cancel := workContext(r)
	defer cancel()
	cancelRequest()

	select {
	case <-ctx.Done():
		t.Fatalf("work context ended with the request: %v", ctx.Err())
	case <-time.After(20 * time.Millisecond):
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > careerWorkTimeout {
		t.Errorf("work context deadline = %v (set %v), want bounded by %v", deadline, ok, careerWorkTimeout)
	}
}
