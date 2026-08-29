// Package objstore is a thin wrapper over the S3 protocol (ADR-018: managed
// S3-compatible service in production, SeaweedFS locally). It carries no
// business rules; keys are chosen by domain packages.
package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

type Client struct {
	mc     *minio.Client
	bucket string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("objstore client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket}, nil
}

// FromEnv builds a client from the standard OBJSTORE_* variables, the same
// five every command reads. An empty access key means anonymous access, which
// is local dev only. Callers still decide whether to EnsureBucket.
func FromEnv() (*Client, error) {
	return New(
		envx.Or("OBJSTORE_ENDPOINT", "localhost:8333"),
		os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"),
		envx.Or("OBJSTORE_BUCKET", "skillhub"),
		os.Getenv("OBJSTORE_SSL") == "1",
	)
}

// EnsureBucket creates the bucket if missing. Dev convenience; production
// buckets are provisioned out of band.
func (c *Client) EnsureBucket(ctx context.Context) error {
	ok, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("objstore bucket check: %w", err)
	}
	if ok {
		return nil
	}
	if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("objstore make bucket: %w", err)
	}
	return nil
}

// MaxObjectBytes bounds what Get will read into memory.
//
// 128 MiB is the largest object any writer in this platform is allowed to
// produce plus room to spare: testlab.MaxTestCaseBytes caps a dataset at 100
// MiB and run.Limits.ArtifactTotalBytes caps a run's outputs at the same, and
// every other ceiling is far below them (delivery.MaxProducedZipBytes is 18 MiB,
// skillpkg.MaxZipBytes 10 MB). The number is repeated here rather than imported
// because this package is generic and must not import a context (ADR-032 §1);
// what it is derived FROM is named above so the derivation is checkable.
//
// Why a bound at all when the write side already has one: nine contexts call
// Get on a request path, one of them in a loop, and "the writers cap it" is a
// transitive argument across several packages. It stops being true the moment
// anything reaches the store another way — clean mode's in-process backend
// takes a PUT from any local process, and llmclient set exactly this bound for
// exactly this reason ("internal is not trusted", MaxResponseBytes).
const MaxObjectBytes = 128 << 20

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("objstore get %s: %w", key, err)
	}
	defer func() { _ = obj.Close() }()
	data, err := readCapped(obj, MaxObjectBytes)
	if err != nil {
		return nil, fmt.Errorf("objstore get %s: %w", key, err)
	}
	return data, nil
}

// readCapped reads r whole, or fails rather than truncating.
//
// The +1 is the whole trick and the reason this is a function shared with the
// in-process backend instead of two inline LimitReaders: io.ReadAll on a plain
// LimitReader returns a SHORT object and no error, so an over-sized object would
// come back as a corrupt package or a hash mismatch somewhere downstream with
// nothing saying why. Reading one byte past the ceiling makes "exactly at the
// limit" legal and "over" loud.
func readCapped(r io.Reader, max int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, fmt.Errorf("object is larger than the %d byte ceiling", max)
	}
	return data, nil
}

// Remove deletes one object. Deleting a key that is not there succeeds, which
// is what makes the retention purge safe to re-run (CORE-007, iron rule 9).
func (c *Client) Remove(ctx context.Context, key string) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("objstore remove %s: %w", key, err)
	}
	return nil
}

// Exists reports whether one object is there, for the reconciler that keeps the
// database's claims about stored bytes honest (04 丙-9).
//
// A definitive 404 is the only "no". Every other failure — a timeout, a refused
// connection, a permissions error — returns an error and never `false`, because
// the caller's response to false is to mark user content as gone, and an
// unreachable store must never look like an empty one.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{}); err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, fmt.Errorf("objstore stat %s: %w", key, err)
	}
	return true, nil
}

// PresignGet and PresignPut mint the short-lived, single-path authorizations of
// SBX-008: one object, one direction, one expiry. They are the only way bytes
// move between the control plane's storage and the execution plane (iron rule
// 2) - the execution node is handed a URL, never a credential and never a
// database connection.
//
// The returned URL is secret material: anyone holding it has exactly the access
// it names until it expires, so it is never logged or traced (iron rule 11).
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("objstore presign get %s: %w", key, err)
	}
	return u.String(), nil
}

func (c *Client) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, c.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("objstore presign put %s: %w", key, err)
	}
	return u.String(), nil
}

func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("objstore put %s: %w", key, err)
	}
	return nil
}
