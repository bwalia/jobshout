package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestBoardStatusForSpecialist(t *testing.T) {
	cases := []struct {
		run  string
		want string
		move bool
	}{
		{"completed", "done", true},
		{"failed", "", false},
		{"cancelled", "", false},
		{"budget_exceeded", "", false},
		{"running", "", false},
		{"queued", "", false},
	}
	for _, tc := range cases {
		got, ok := boardStatusForSpecialist(tc.run)
		if ok != tc.move || got != tc.want {
			t.Fatalf("%s: got (%q, %v), want (%q, %v)", tc.run, got, ok, tc.want, tc.move)
		}
	}
}

type transStub struct {
	called bool
	id     uuid.UUID
	status string
}

func (t *transStub) Transition(_ context.Context, id uuid.UUID, status string, _ *uuid.UUID) error {
	t.called = true
	t.id = id
	t.status = status
	return nil
}

func TestSyncSpecialistBoard(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	stub := &transStub{}
	syncSpecialistBoard(ctx, stub, &id, "completed")
	if !stub.called || stub.id != id || stub.status != "done" {
		t.Fatalf("completed should move to done: %+v", stub)
	}

	stub = &transStub{}
	syncSpecialistBoard(ctx, stub, &id, "failed")
	if stub.called {
		t.Fatal("failed must not transition the board task")
	}

	stub = &transStub{}
	syncSpecialistBoard(ctx, stub, nil, "completed")
	if stub.called {
		t.Fatal("nil task id must be a no-op")
	}

	syncSpecialistBoard(ctx, nil, &id, "completed")
}
