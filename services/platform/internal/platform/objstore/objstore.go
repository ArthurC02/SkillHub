// Package objstore is a thin wrapper over the S3 protocol (ADR-018: managed
// S3-compatible service in production, SeaweedFS locally). It carries no
// business rules; keys are chosen by domain packages.
package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
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

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("objstore get %s: %w", key, err)
	}
	defer func() { _ = obj.Close() }()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("objstore get %s: %w", key, err)
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
