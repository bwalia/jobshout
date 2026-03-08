package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
)

// MarketplaceHandler handles agent marketplace endpoints.
type MarketplaceHandler struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewMarketplaceHandler constructs a MarketplaceHandler.
func NewMarketplaceHandler(pool *pgxpool.Pool, logger *zap.Logger) *MarketplaceHandler {
	return &MarketplaceHandler{pool: pool, logger: logger}
}

// marketplaceAgentRow is the DB row for a marketplace agent.
type marketplaceAgentRow struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	ModelProvider string  `json:"model_provider"`
	ModelName     string  `json:"model_name"`
	SystemPrompt  string  `json:"system_prompt"`
	AuthorID      *string `json:"author_id"`
	Downloads     int     `json:"downloads"`
	Rating        float64 `json:"rating"`
	IsPublic      bool    `json:"is_public"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// List returns marketplace agents with optional category filter.
func (h *MarketplaceHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")

	query := `SELECT id, name, description, category, model_provider, model_name, system_prompt,
		author_id, downloads, rating, is_public, created_at, updated_at
		FROM marketplace_agents WHERE is_public = true`
	args := []any{}

	if category != "" {
		query += " AND category = $1"
		args = append(args, category)
	}
	query += " ORDER BY downloads DESC LIMIT 50"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		h.logger.Error("failed to list marketplace agents", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list marketplace agents")
		return
	}
	defer rows.Close()

	agents := []marketplaceAgentRow{}
	for rows.Next() {
		var a marketplaceAgentRow
		if err := rows.Scan(
			&a.ID, &a.Name, &a.Description, &a.Category, &a.ModelProvider,
			&a.ModelName, &a.SystemPrompt, &a.AuthorID, &a.Downloads,
			&a.Rating, &a.IsPublic, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			h.logger.Error("failed to scan marketplace agent", zap.Error(err))
			continue
		}
		agents = append(agents, a)
	}

	RespondJSON(w, http.StatusOK, agents)
}

// GetByID returns a single marketplace agent.
func (h *MarketplaceHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "agentID")

	var a marketplaceAgentRow
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, name, description, category, model_provider, model_name, system_prompt,
			author_id, downloads, rating, is_public, created_at, updated_at
			FROM marketplace_agents WHERE id = $1`, id,
	).Scan(
		&a.ID, &a.Name, &a.Description, &a.Category, &a.ModelProvider,
		&a.ModelName, &a.SystemPrompt, &a.AuthorID, &a.Downloads,
		&a.Rating, &a.IsPublic, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		RespondError(w, http.StatusNotFound, "marketplace agent not found")
		return
	}

	RespondJSON(w, http.StatusOK, a)
}

// Import copies a marketplace agent into the user's org as a new agent.
func (h *MarketplaceHandler) Import(w http.ResponseWriter, r *http.Request) {
	marketplaceID := chi.URLParam(r, "agentID")
	orgID, _ := r.Context().Value(middleware.ContextKeyOrgID).(string)

	if orgID == "" {
		RespondError(w, http.StatusUnauthorized, "missing organization context")
		return
	}

	// Fetch marketplace agent
	var name, description, modelProvider, modelName, systemPrompt string
	err := h.pool.QueryRow(r.Context(),
		`SELECT name, description, model_provider, model_name, system_prompt
			FROM marketplace_agents WHERE id = $1`, marketplaceID,
	).Scan(&name, &description, &modelProvider, &modelName, &systemPrompt)
	if err != nil {
		RespondError(w, http.StatusNotFound, "marketplace agent not found")
		return
	}

	// Create the agent in the user's org
	var newID string
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO agents (name, description, model_provider, model_name, system_prompt, org_id, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'idle')
			RETURNING id`,
		name, description, modelProvider, modelName, systemPrompt, orgID,
	).Scan(&newID)
	if err != nil {
		h.logger.Error("failed to import marketplace agent", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to import agent")
		return
	}

	// Increment download counter
	_, _ = h.pool.Exec(r.Context(),
		`UPDATE marketplace_agents SET downloads = downloads + 1 WHERE id = $1`, marketplaceID)

	RespondJSON(w, http.StatusCreated, map[string]string{
		"id":      newID,
		"message": "agent imported successfully",
	})
}
