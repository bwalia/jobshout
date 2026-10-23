-- Generated images (blog covers, illustrations, rendered videos) for a ring
-- with no MinIO. Shared by every API replica and kept across deploys, unlike
-- the pod-local directory it replaces. Keys are the object-store keys
-- (<org>/<yyyy>/<mm>/<uuid>.<ext>), served at /api/v1/images/file/<key>.
CREATE TABLE IF NOT EXISTS image_blobs (
    key        TEXT PRIMARY KEY,
    data       BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
