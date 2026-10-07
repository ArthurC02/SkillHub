package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc     *minio.Client
	bucket string
}

const (
	storeAnswerWait       = 20 * time.Second
	storeAttempts         = 3
	storeIdleConnsPerHost = 64
)

func storeTransport(useSSL bool) (*http.Transport, error) {
	transport, err := minio.DefaultTransport(useSSL)
	if err != nil {
		return nil, err
	}
	transport.ResponseHeaderTimeout = storeAnswerWait
	transport.MaxIdleConnsPerHost = storeIdleConnsPerHost
	return transport, nil
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	transport, err := storeTransport(useSSL)
	if err != nil {
		return nil, fmt.Errorf("objstore transport: %w", err)
	}
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:      credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:     useSSL,
		Transport:  transport,
		MaxRetries: storeAttempts,
	})
	if err != nil {
		return nil, fmt.Errorf("objstore client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	ok, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("objstore bucket check: %w", err)
	}
	if ok {
		return nil
	}
	if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
		if created, checkErr := c.mc.BucketExists(ctx, c.bucket); checkErr == nil && created {
			return nil
		}
		return fmt.Errorf("objstore make bucket: %w", err)
	}
	return nil
}

// A HEAD answer carries no body, so a missing bucket and a missing object are
// the same 404; only asking for the bucket tells them apart.
func (c *Client) requireBucket(ctx context.Context) error {
	ok, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("objstore bucket check: %w", err)
	}
	if !ok {
		return fmt.Errorf("objstore bucket %s does not exist", c.bucket)
	}
	return nil
}

const MaxObjectBytes = 128 << 20

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	data, found, err := c.GetIfPresent(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("objstore get %s: no object with that key: %w", key, fs.ErrNotExist)
	}
	return data, nil
}

func (c *Client) GetIfPresent(ctx context.Context, key string) ([]byte, bool, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("objstore get %s: %w", key, err)
	}
	defer func() { _ = obj.Close() }()
	sizeHint := int64(-1)
	if info, err := obj.Stat(); err == nil {
		sizeHint = info.Size
	}
	data, err := readCapped(obj, MaxObjectBytes, sizeHint)
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("objstore get %s: %w", key, err)
	}
	return data, true, nil
}

func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, openFailure(key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, 0, openFailure(key, err)
	}
	return obj, info.Size, nil
}

func openFailure(key string, err error) error {
	if isNotFound(err) {
		err = fs.ErrNotExist
	}
	return fmt.Errorf("objstore open %s: %w", key, err)
}

func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.StatusCode == http.StatusNotFound && resp.Code != "NoSuchBucket"
}

// readCapped reads one byte past the ceiling: a plain LimitReader ends
// silently short at the limit, with no error, so the extra byte is what turns
// "over the ceiling" into a reported error instead.
func readCapped(r io.Reader, ceiling int, sizeHint int64) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, int(min(max(sizeHint, 0), int64(ceiling)))+bytes.MinRead))
	if _, err := buf.ReadFrom(io.LimitReader(r, int64(ceiling)+1)); err != nil {
		return nil, err
	}
	if buf.Len() > ceiling {
		return nil, fmt.Errorf("object is larger than the %d byte ceiling", ceiling)
	}
	return buf.Bytes(), nil
}

func (c *Client) Remove(ctx context.Context, key string) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("objstore remove %s: %w", key, err)
	}
	return nil
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if _, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{}); err != nil {
		if isNotFound(err) {
			return false, c.requireBucket(ctx)
		}
		return false, fmt.Errorf("objstore stat %s: %w", key, err)
	}
	return true, nil
}

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

func withinObjectCeiling[N int | int64](size, max N) error {
	if size > max {
		return fmt.Errorf("object is larger than the %d byte ceiling", max)
	}
	return nil
}

func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	return c.PutFrom(ctx, key, bytes.NewReader(data), int64(len(data)))
}

func (c *Client) PutFrom(ctx context.Context, key string, content io.Reader, size int64) error {
	if err := withinObjectCeiling(size, MaxObjectBytes); err != nil {
		return fmt.Errorf("objstore put %s: %w", key, err)
	}
	_, err := c.mc.PutObject(ctx, c.bucket, key, content, size, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("objstore put %s: %w", key, err)
	}
	return nil
}
