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

// TestInProcess exercises all seven Client methods against the in-process S3
// stand-in, including hitting the two presigned URLs with a plain HTTP client
// the way the sandbox does - not merely asserting that a URL string comes
// back.
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
	if err := client.EnsureBucket(ctx); err != nil { // idempotent
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

	// PresignGet must round-trip through the actual URL - hit it with a plain
	// http.Get like the sandbox does, not just assert the string is non-empty.
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

	// PresignPut, likewise, hit with a plain http.Put and confirm it lands.
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
	// Removing an already-absent key must still succeed (iron rule 9: destroy
	// is safe to repeat).
	if err := client.Remove(ctx, "greeting.txt"); err != nil {
		t.Fatalf("Remove (repeat, already absent): %v", err)
	}
}

// TestInProcessDoesNotAuthorize records, as a passing assertion, the security
// gap documented on inProcessBackend's doc comment: this stand-in accepts a
// tampered signature, a stripped signature, an already-expired presign, and a
// GET-scoped presign reused for a PUT. All four succeed here by design - if
// any of them ever starts failing, this backend has quietly grown real
// authorization and every caller relying on "unauthenticated by design" (this
// is not SBX-008 evidence) needs to know.
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
	time.Sleep(3 * time.Second) // let the 1s TTL lapse

	// 1. Tampered signature on the GET URL: flip the last 4 hex characters.
	tampered := getURL[:len(getURL)-4] + "dead"
	assertStatus(t, "tampered signature", http.MethodGet, tampered, nil, http.StatusOK)

	// 2. Signature stripped entirely.
	sigIdx := strings.Index(getURL, "&X-Amz-Signature=")
	if sigIdx < 0 {
		t.Fatal("presigned GET URL has no &X-Amz-Signature= to strip")
	}
	assertStatus(t, "stripped signature", http.MethodGet, getURL[:sigIdx], nil, http.StatusOK)

	// 3. Expired presign (Expires already in the past).
	assertStatus(t, "expired presign PUT", http.MethodPut, expiredPutURL,
		bytes.NewReader([]byte("expired-write")), http.StatusOK)

	// 4. A GET-scoped presign string, hit with PUT instead of GET: it actually
	// overwrites the object.
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

// The three ceilings clean mode needs, because in clean mode this backend is not
// a test double: cmd/api serves it as the deployment's real object store, on a
// loopback listener any other local process can reach.

// A chunk header is a number the caller chose, and it used to be passed straight
// to make([]byte, size). "7fffffffffffffff" asks for 8 exabytes in one
// allocation.
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

// Not authentication -- nothing here verifies a signature, and the doc comment
// says so -- but a caller now has to HOLD the key this process generated.
// Without it, every object on the machine was readable and writable by anything
// that could guess a key.
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

	// The real client, which does hold the key, must be unaffected.
	if got, err := client.Get(ctx, "secret.txt"); err != nil || string(got) != "private" {
		t.Fatalf("the configured client can no longer read its own object: %q %v", got, err)
	}
}

// inProcessEndpoint recovers the listener address and the key from the wired
// client, so a test can speak to the backend the way another local process
// would. Both come out of a presigned URL, which is the only place the client
// exposes either.
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
