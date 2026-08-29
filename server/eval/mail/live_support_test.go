//go:build evallive

package mail_eval

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// liveMailService is the wiring seam for the Tier 2 suite.
//
// It is deliberately unimplemented: standing up the real service needs a
// database pool, the research service and the model router, and the shape of
// that wiring belongs with whoever runs the suite on the workstation rather
// than hard-coded here. Returning an error makes the live suite skip with a
// clear reason instead of failing.
func liveMailService(context.Context, uuid.UUID) (service.MailService, func(), error) {
	return nil, func() {}, errors.New("liveMailService is not wired: point it at the workstation's service graph to run tier 2")
}

func defaultPage() model.PaginationParams {
	return model.PaginationParams{Page: 1, PerPage: 50}
}
