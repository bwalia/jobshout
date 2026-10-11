package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/propertypa"
)

// PropertyPAInstaller is the slice of propertypa.Installer the handler uses.
type PropertyPAInstaller interface {
	Install(ctx context.Context, orgID, userID uuid.UUID) (*propertypa.InstallResult, error)
	List(ctx context.Context, orgID uuid.UUID) ([]propertypa.Status, error)
}

// PropertyPAHandler installs and lists the Property PA agents of the caller's org.
type PropertyPAHandler struct {
	inst   PropertyPAInstaller
	logger *zap.Logger
}

// NewPropertyPAHandler builds the handler.
func NewPropertyPAHandler(inst PropertyPAInstaller, logger *zap.Logger) *PropertyPAHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PropertyPAHandler{inst: inst, logger: logger}
}

// Install creates or upgrades the org's Seller PA and Manager PA (org:manage).
func (h *PropertyPAHandler) Install(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user")
		return
	}
	res, err := h.inst.Install(r.Context(), orgID, userID)
	if err != nil {
		h.logger.Error("property pa install failed", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to install the Property PA agents")
		return
	}
	RespondJSON(w, http.StatusOK, res)
}

// List reports the org's PA agents: ids, installed and available prompt versions.
func (h *PropertyPAHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	st, err := h.inst.List(r.Context(), orgID)
	if err != nil {
		h.logger.Error("property pa list failed", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list the Property PA agents")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{
		"agents":          st,
		"output_contract": propertypa.OutputContract,
		"skills":          propertypaSkillSlugs(),
	})
}

func propertypaSkillSlugs() []string {
	var out []string
	for _, s := range propertypa.Skills() {
		out = append(out, s.Slug)
	}
	return out
}
