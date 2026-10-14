package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// stubSessionRepo holds one session; calls it doesn't expect panic via the
// nil embedded interface.
type stubSessionRepo struct {
	repository.SessionRepository
	s       *model.Session
	deleted bool
}

func (r *stubSessionRepo) GetByID(_ context.Context, id uuid.UUID) (*model.Session, error) {
	return r.s, nil
}

func (r *stubSessionRepo) Delete(_ context.Context, _ uuid.UUID) error {
	r.deleted = true
	return nil
}

func TestSessionRoutes_OtherOrgIsNotFound(t *testing.T) {
	owner, caller := uuid.New(), uuid.New()
	sess := &model.Session{ID: uuid.New(), OrgID: owner}

	for _, tc := range []struct {
		org  uuid.UUID
		want int
	}{{caller, http.StatusNotFound}, {owner, http.StatusNoContent}} {
		repo := &stubSessionRepo{s: sess}
		h := NewSessionHandler(repo)
		r := chi.NewRouter()
		r.Delete("/sessions/{sessionID}", h.Delete)

		req := httptest.NewRequest(http.MethodDelete, "/sessions/"+sess.ID.String(), nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyOrgID, tc.org.String()))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != tc.want {
			t.Fatalf("org %s: status %d, want %d", tc.org, rr.Code, tc.want)
		}
		if repo.deleted != (tc.want == http.StatusNoContent) {
			t.Fatalf("org %s: deleted=%v", tc.org, repo.deleted)
		}
	}
}
