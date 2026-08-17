// Package imagestore puts generated images somewhere they can be served from.
//
// It is separate from imagegen because producing a picture and keeping one are
// different concerns with different failure modes: a model that is asleep and a
// bucket that is full are not the same problem, and a caller usually wants to
// know which it hit.
package imagestore

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// URLPrefix is the API path generated images are served under. Stored URLs are
// relative to the host so that an image survives the platform being reached on
// a different hostname per ring — an absolute URL baked in at int would point
// at int from prod.
const URLPrefix = "/api/v1/images/file/"

// Store keeps generated images.
type Store interface {
	// Put stores png and returns the URL it will be served from.
	Put(ctx context.Context, orgID uuid.UUID, png []byte) (string, error)
	// Get returns a stored image by the object key embedded in its URL.
	Get(ctx context.Context, key string) ([]byte, error)
}

// MinIOStore stores images in an S3-compatible bucket.
type MinIOStore struct {
	client *minio.Client
	bucket string

	// The bucket is created on first use rather than at startup, so a server
	// whose object storage arrives late still works, and one that never
	// generates an image never creates a bucket it does not need. once, so
	// concurrent first writes do not race to create it.
	ensure    sync.Once
	ensureErr error
}

// NewMinIOStore builds a store over an existing MinIO client.
func NewMinIOStore(client *minio.Client, bucket string) *MinIOStore {
	return &MinIOStore{client: client, bucket: bucket}
}

func (s *MinIOStore) ensureBucket(ctx context.Context) error {
	s.ensure.Do(func() {
		exists, err := s.client.BucketExists(ctx, s.bucket)
		if err != nil {
			s.ensureErr = fmt.Errorf("imagestore: check bucket %q: %w", s.bucket, err)
			return
		}
		if exists {
			return
		}
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			// Another process may have won the race between the check and the
			// creation. That is success, not failure.
			if exists, checkErr := s.client.BucketExists(ctx, s.bucket); checkErr == nil && exists {
				return
			}
			s.ensureErr = fmt.Errorf("imagestore: create bucket %q: %w", s.bucket, err)
		}
	})
	return s.ensureErr
}

// Put implements Store.
//
// Keys are org-scoped and date-partitioned: the org prefix keeps one tenant's
// images from being guessable by iterating another's, and the date keeps a
// bucket that grows for years from becoming one flat directory of a million
// objects.
func (s *MinIOStore) Put(ctx context.Context, orgID uuid.UUID, png []byte) (string, error) {
	if len(png) == 0 {
		return "", fmt.Errorf("imagestore: refusing to store an empty image")
	}
	if err := s.ensureBucket(ctx); err != nil {
		return "", err
	}

	key := fmt.Sprintf("%s/%s/%s.png", orgID, time.Now().UTC().Format("2006/01"), uuid.New())

	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(png), int64(len(png)),
		minio.PutObjectOptions{ContentType: "image/png"})
	if err != nil {
		return "", fmt.Errorf("imagestore: store image: %w", err)
	}

	return URLPrefix + key, nil
}

// Get implements Store.
func (s *MinIOStore) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("imagestore: fetch image: %w", err)
	}
	defer obj.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(obj); err != nil {
		return nil, fmt.Errorf("imagestore: read image: %w", err)
	}
	return buf.Bytes(), nil
}
