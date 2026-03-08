package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
)

// UploadHandler handles file upload endpoints using MinIO.
type UploadHandler struct {
	minioClient  *minio.Client
	bucketAvatars string
	logger       *zap.Logger
}

// NewUploadHandler creates a new UploadHandler.
func NewUploadHandler(minioClient *minio.Client, bucketAvatars string, logger *zap.Logger) *UploadHandler {
	return &UploadHandler{
		minioClient:  minioClient,
		bucketAvatars: bucketAvatars,
		logger:       logger,
	}
}

// UploadAvatar handles POST /uploads/avatar
// Accepts multipart form with a "file" field.
func (h *UploadHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		RespondError(w, http.StatusUnauthorized, "missing user context")
		return
	}

	// Limit upload to 5MB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		RespondError(w, http.StatusBadRequest, "failed to read uploaded file: "+err.Error())
		return
	}
	defer file.Close()

	// Validate content type
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		RespondError(w, http.StatusBadRequest, "only image files are allowed")
		return
	}

	ext := path.Ext(header.Filename)
	if ext == "" {
		ext = ".png"
	}
	objectName := fmt.Sprintf("%s/%s%s", userID, uuid.New().String(), ext)

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := h.minioClient.BucketExists(ctx, h.bucketAvatars)
	if err != nil {
		h.logger.Error("failed to check bucket", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "storage error")
		return
	}
	if !exists {
		if err := h.minioClient.MakeBucket(ctx, h.bucketAvatars, minio.MakeBucketOptions{}); err != nil {
			h.logger.Error("failed to create bucket", zap.Error(err))
			RespondError(w, http.StatusInternalServerError, "storage error")
			return
		}
	}

	info, err := h.minioClient.PutObject(ctx, h.bucketAvatars, objectName, file, header.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		h.logger.Error("failed to upload to minio", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to upload file")
		return
	}

	// Discard any remaining body to prevent connection issues
	io.Copy(io.Discard, r.Body)

	avatarURL := fmt.Sprintf("/api/v1/uploads/avatar/%s", objectName)

	RespondJSON(w, http.StatusOK, map[string]any{
		"url":  avatarURL,
		"size": info.Size,
		"key":  info.Key,
	})
}

// ServeAvatar handles GET /uploads/avatar/* — serves avatar images from MinIO.
func (h *UploadHandler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	// Extract everything after /uploads/avatar/
	objectName := strings.TrimPrefix(r.URL.Path, "/api/v1/uploads/avatar/")
	if objectName == "" {
		RespondError(w, http.StatusBadRequest, "missing object key")
		return
	}

	obj, err := h.minioClient.GetObject(r.Context(), h.bucketAvatars, objectName, minio.GetObjectOptions{})
	if err != nil {
		RespondError(w, http.StatusNotFound, "file not found")
		return
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err != nil {
		RespondError(w, http.StatusNotFound, "file not found")
		return
	}

	w.Header().Set("Content-Type", stat.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, obj)
}
