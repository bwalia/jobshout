package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// ImageRepository records the images the platform has generated.
//
// One table for all of them — article covers, agent tool calls and manual
// requests alike — because the questions worth asking of the record do not care
// which route produced the image.
type ImageRepository interface {
	Create(ctx context.Context, img *model.GeneratedImage) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, limit int) ([]model.GeneratedImage, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.GeneratedImage, error)
	// Routes returns the org's image model per task (empty when it chose none).
	Routes(ctx context.Context, orgID uuid.UUID) (map[string]model.ImageRoute, error)
	// PutRoutes replaces the org's image model per task.
	PutRoutes(ctx context.Context, orgID uuid.UUID, routes map[string]model.ImageRoute, updatedBy *uuid.UUID) error
}

type imageRepository struct {
	pool *pgxpool.Pool
}

// NewImageRepository builds an ImageRepository over the connection pool.
func NewImageRepository(pool *pgxpool.Pool) ImageRepository {
	return &imageRepository{pool: pool}
}

const generatedImageColumns = `
	id, org_id, created_by, prompt, provider, model, seed, width, height,
	url, source, blog_run_id, duration_ms, created_at`

func scanGeneratedImage(row pgx.Row) (*model.GeneratedImage, error) {
	var img model.GeneratedImage
	// url is nullable: an image generated without object storage configured is
	// still worth recording, it just has nowhere to be served from.
	var url *string
	err := row.Scan(
		&img.ID, &img.OrgID, &img.CreatedBy, &img.Prompt, &img.Provider, &img.Model,
		&img.Seed, &img.Width, &img.Height, &url, &img.Source, &img.BlogRunID, &img.DurationMS, &img.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if url != nil {
		img.URL = *url
	}
	return &img, nil
}

func (r *imageRepository) Create(ctx context.Context, img *model.GeneratedImage) error {
	if img.ID == uuid.Nil {
		img.ID = uuid.New()
	}
	if img.Source == "" {
		img.Source = model.ImageSourceManual
	}

	// An empty URL is written as NULL rather than "", so "no stored bytes" has
	// one representation in the column rather than two.
	var url *string
	if img.URL != "" {
		url = &img.URL
	}

	const sql = `
		INSERT INTO generated_images
		    (id, org_id, created_by, prompt, provider, model, seed, width, height, url, source, blog_run_id, duration_ms, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13, NOW())
		RETURNING created_at`

	err := r.pool.QueryRow(ctx, sql,
		img.ID, img.OrgID, img.CreatedBy, img.Prompt, img.Provider, img.Model,
		img.Seed, img.Width, img.Height, url, img.Source, img.BlogRunID, img.DurationMS,
	).Scan(&img.CreatedAt)
	if err != nil {
		return fmt.Errorf("image_repo: insert image: %w", err)
	}
	return nil
}

// defaultImageListLimit bounds an unbounded listing. Images are large to
// display, so a page of them is a page, not a year of them.
const defaultImageListLimit = 50

func (r *imageRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, limit int) ([]model.GeneratedImage, error) {
	if limit <= 0 || limit > 200 {
		limit = defaultImageListLimit
	}

	sql := `SELECT ` + generatedImageColumns + `
		FROM generated_images WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("image_repo: list images: %w", err)
	}
	defer rows.Close()

	images := make([]model.GeneratedImage, 0)
	for rows.Next() {
		img, err := scanGeneratedImage(rows)
		if err != nil {
			return nil, fmt.Errorf("image_repo: scan image: %w", err)
		}
		images = append(images, *img)
	}
	return images, rows.Err()
}

func (r *imageRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.GeneratedImage, error) {
	sql := `SELECT ` + generatedImageColumns + ` FROM generated_images WHERE id = $1`

	img, err := scanGeneratedImage(r.pool.QueryRow(ctx, sql, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("image_repo: get image: %w", err)
	}
	return img, nil
}

func (r *imageRepository) Routes(ctx context.Context, orgID uuid.UUID) (map[string]model.ImageRoute, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT routes FROM org_image_routing WHERE org_id = $1`, orgID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]model.ImageRoute{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("org_image_routing: get: %w", err)
	}
	routes := map[string]model.ImageRoute{}
	if err := json.Unmarshal(raw, &routes); err != nil {
		return nil, fmt.Errorf("org_image_routing: decode: %w", err)
	}
	return routes, nil
}

func (r *imageRepository) PutRoutes(ctx context.Context, orgID uuid.UUID, routes map[string]model.ImageRoute, updatedBy *uuid.UUID) error {
	if routes == nil {
		routes = map[string]model.ImageRoute{}
	}
	raw, err := json.Marshal(routes)
	if err != nil {
		return fmt.Errorf("org_image_routing: encode: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO org_image_routing (org_id, routes, updated_by) VALUES ($1, $2, $3)
		ON CONFLICT (org_id) DO UPDATE
		SET routes = EXCLUDED.routes, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		orgID, raw, updatedBy)
	if err != nil {
		return fmt.Errorf("org_image_routing: put: %w", err)
	}
	return nil
}
