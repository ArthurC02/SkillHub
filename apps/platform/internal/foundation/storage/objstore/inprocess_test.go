package objstore

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestInProcess(t *testing.T) {
	client, stop, err := NewInProcess("test-bucket")
	if err != nil {
		t.Fatalf("NewInProcess: %v", err)
	}
	defer stop()

	ctx := context.Background()

	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket (second call): %v", err)
	}

	exists, err := client.Exists(ctx, "missing-key")
	if err != nil {
		t.Fatalf("Exists(missing): %v", err)
	}
	if exists {
		t.Fatal("Exists(missing) = true, want false")
	}
	if _, err := client.Get(ctx, "missing-key"); err == nil {
		t.Fatal("Get(missing): want error, got nil")
	}

	payload := []byte("hello in-process s3")
	if err := client.Put(ctx, "greeting.txt", payload); err != nil {
		t.Fatalf("Put: %v", err)
	}

	exists, err = client.Exists(ctx, "greeting.txt")
	if err != nil {
		t.Fatalf("Exists(present): %v", err)
	}
	if !exists {
		t.Fatal("Exists(present) = false, want true")
	}

	got, err := client.Get(ctx, "greeting.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Get = %q, want %q", got, payload)
	}

	getURL, err := client.PresignGet(ctx, "greeting.txt", time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	resp, err := http.Get(getURL)
	if err != nil {
		t.Fatalf("http.Get(presigned): %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read presigned GET body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned GET status = %d, want 200", resp.StatusCode)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("presigned GET body = %q, want %q", body, payload)
	}

	putURL, err := client.PresignPut(ctx, "uploaded.txt", time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	putPayload := []byte("written through the presigned URL")
	putReq, err := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(putPayload))
	if err != nil {
		t.Fatalf("NewRequest(PUT): %v", err)
	}
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("http.Put(presigned): %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("presigned PUT status = %d, want 200", putResp.StatusCode)
	}

	got, err = client.Get(ctx, "uploaded.txt")
	if err != nil {
		t.Fatalf("Get(uploaded.txt): %v", err)
	}
	if !bytes.Equal(got, putPayload) {
		t.Fatalf("Get(uploaded.txt) = %q, want %q", got, putPayload)
	}

	if err := client.Remove(ctx, "greeting.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	exists, err = client.Exists(ctx, "greeting.txt")
	if err != nil {
		t.Fatalf("Exists(after remove): %v", err)
	}
	if exists {
		t.Fatal("Exists(after remove) = true, want false")
	}

	if err := client.Remove(ctx, "greeting.txt"); err != nil {
		t.Fatalf("Remove (repeat, already absent): %v", err)
	}
}

func TestInProcessDoesNotAuthorize(t *testing.T) {
	client, stop, err := NewInProcess("test-bucket")
	if err != nil {
		t.Fatalf("NewInProcess: %v", err)
	}
	defer stop()
	ctx := context.Background()
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := client.Put(ctx, "target.txt", []byte("original")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	getURL, err := client.PresignGet(ctx, "target.txt", time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	expiredPutURL, err := client.PresignPut(ctx, "target.txt", time.Second)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	time.Sleep(3 * time.Second)

	tampered := getURL[:len(getURL)-4] + "dead"
	assertStatus(t, "tampered signature", http.MethodGet, tampered, nil, http.StatusOK)

	sigIdx := strings.Index(getURL, "&X-Amz-Signature=")
	if sigIdx < 0 {
		t.Fatal("presigned GET URL has no &X-Amz-Signature= to strip")
	}
	assertStatus(t, "stripped signature", http.MethodGet, getURL[:sigIdx], nil, http.StatusOK)

	assertStatus(t, "expired presign PUT", http.MethodPut, expiredPutURL,
		bytes.NewReader([]byte("expired-write")), http.StatusOK)

	overwrite := []byte("overwritten via a GET-scoped URL")
	assertStatus(t, "GET-scoped PUT", http.MethodPut, getURL, bytes.NewReader(overwrite), http.StatusOK)

	got, err := client.Get(ctx, "target.txt")
	if err != nil {
		t.Fatalf("Get after GET-scoped overwrite: %v", err)
	}
	if !bytes.Equal(got, overwrite) {
		t.Fatalf("object after GET-scoped PUT = %q, want %q (the overwrite must actually land)", got, overwrite)
	}
}

func assertStatus(t *testing.T, label, method, url string, body io.Reader, want int) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("%s: NewRequest: %v", label, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s: status = %d, want %d (unenforced, as documented on inProcessBackend)", label, resp.StatusCode, want)
	}
}

func TestAnOversizedChunkHeaderIsRefusedRatherThanAllocated(t *testing.T) {
	client, stop, err := NewInProcess("test-bucket")
	if err != nil {
		t.Fatalf("NewInProcess: %v", err)
	}
	defer stop()
	if err := client.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	endpoint, key := inProcessEndpoint(t, client)

	body := "7fffffffffffffff;chunk-signature=0\r\n"
	req, err := http.NewRequest(http.MethodPut, endpoint+"/test-bucket/huge.bin", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Amz-Content-Sha256", streamingSignAlgorithm)
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+key+"/x")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a chunk header of 8 EB answered %d, want 400", resp.StatusCode)
	}
}

func TestARequestWithoutTheProcessKeyIsRefused(t *testing.T) {
	client, stop, err := NewInProcess("test-bucket")
	if err != nil {
		t.Fatalf("NewInProcess: %v", err)
	}
	defer stop()
	ctx := context.Background()
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := client.Put(ctx, "secret.txt", []byte("private")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	endpoint, _ := inProcessEndpoint(t, client)

	for _, tc := range []struct{ name, auth string }{
		{name: "no Authorization at all", auth: ""},
		{name: "a made-up key", auth: "AWS4-HMAC-SHA256 Credential=inprocess/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, endpoint+"/test-bucket/secret.txt", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == http.StatusOK {
				t.Error("a caller with no process key read an object")
			}
		})
	}

	if got, err := client.Get(ctx, "secret.txt"); err != nil || string(got) != "private" {
		t.Fatalf("the configured client can no longer read its own object: %q %v", got, err)
	}
}

func inProcessEndpoint(t *testing.T, c *Client) (endpoint, key string) {
	t.Helper()
	raw, err := c.PresignGet(context.Background(), "probe", time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	cred := u.Query().Get("X-Amz-Credential")
	key, _, _ = strings.Cut(cred, "/")
	if key == "" {
		t.Fatalf("no X-Amz-Credential in %q", raw)
	}
	return u.Scheme + "://" + u.Host, key
}
