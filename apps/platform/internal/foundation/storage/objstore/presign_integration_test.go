package objstore

// SBX-008's short-lived grant, measured against a real S3 implementation.
//
// 02:PORT-009. The in-process backend next door (inprocess.go) says in its own
// header that it verifies nothing — no signature, no expiry, no method binding —
// and TestInProcessDoesNotAuthorize asserts exactly that. So the whole of this
// repository's evidence for "a pre-signed URL is a short-lived, single-object,
// single-direction authorization" has to live here, on a store that actually
// checks. Without this file the coverage of that claim is zero, hidden behind a
// stand-in that answers 200 to every attack on it.
//
// Three properties, and nothing else in this file is the point:
//
//  1. an expired ticket stops working,
//  2. a tampered signature is refused,
//  3. a GET ticket cannot PUT — and the object is still what it was afterwards.
//
// Running it locally: `task dev:core` starts SeaweedFS from
// infra/compose/docker-compose.yml, then
//
//	SKILLHUB_TEST_OBJSTORE_ENDPOINT=127.0.0.1:8333 go test ./internal/foundation/storage/objstore/
//
// Unset the endpoint and these tests skip, exactly like the database ones. CI
// sets SKILLHUB_REQUIRE_OBJSTORE=1, which turns the skip into a failure — the
// death this requirement names is not going red, it is being quietly skipped in
// some later CI edit, so the switch and devctl's require-objstore-guard check
// exist to make that edit loud.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

const (
	objstoreEndpointEnv = "SKILLHUB_TEST_OBJSTORE_ENDPOINT"

	// The committed local dev identity from infra/compose/seaweedfs-s3.json,
	// same class as the postgres skillhub/skillhub pair. Overridable so the same
	// test can be pointed at another S3 service; production credentials never
	// come near this file.
	devAccessKey = "skillhubdev"
	devSecretKey = "skillhubdevsecret"

	// Its own bucket: this test writes and (via a rejected PUT) tries to
	// overwrite objects, and must not do that in whatever bucket a developer
	// happens to have their dev data in.
	testBucket = "skillhub-presign-test"
)

var presignStore *Client

func TestMain(m *testing.M) {
	endpoint := os.Getenv(objstoreEndpointEnv)
	if endpoint == "" {
		// 02:PORT-009, the same fail-closed shape as 02:PORT-004's
		// SKILLHUB_REQUIRE_DB. An object store that never came up, or an env
		// block dropped from a workflow, would otherwise remove the only
		// assertions this repository has about short-lived authorization and
		// still print ok.
		// The literal, not a constant: devctl's require-objstore-guard looks for
		// this exact call after stripping comments, the same way
		// require_db_guard.go looks for its own.
		if os.Getenv("SKILLHUB_REQUIRE_OBJSTORE") == "1" {
			fmt.Fprintf(os.Stderr,
				"SKILLHUB_REQUIRE_OBJSTORE=1 but %s is unset; this run would have skipped every short-lived authorization test (SBX-008) and still reported success\n",
				objstoreEndpointEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	client, err := New(endpoint,
		envx.Or("SKILLHUB_TEST_OBJSTORE_ACCESS_KEY", devAccessKey),
		envx.Or("SKILLHUB_TEST_OBJSTORE_SECRET_KEY", devSecretKey),
		testBucket, os.Getenv("SKILLHUB_TEST_OBJSTORE_SSL") == "1")
	if err != nil {
		panic(err)
	}
	if err := client.EnsureBucket(context.Background()); err != nil {
		panic(err)
	}
	presignStore = client
	os.Exit(m.Run())
}

func requirePresignStore(t *testing.T) *Client {
	t.Helper()
	if presignStore == nil {
		t.Skipf("%s not set; skipping the real object store tests", objstoreEndpointEnv)
	}
	return presignStore
}

// fetch performs one raw HTTP request against a pre-signed URL, the way the
// execution plane does: no credentials, no client library, just the URL.
func fetch(t *testing.T, method, raw string, body []byte) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, raw, reader)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, got
}

func TestPresignedGrantIsShortLivedUnforgeableAndSingleDirection(t *testing.T) {
	store := requirePresignStore(t)
	ctx := t.Context()

	key := fmt.Sprintf("presign/%d.bin", time.Now().UnixNano())
	want := []byte("the object SBX-008 grants access to")
	if err := store.Put(ctx, key, want); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Remove(context.WithoutCancel(ctx), key) })

	// The control. Without it every assertion below could be satisfied by a
	// store that refuses everything, including a store that is not there.
	valid, err := store.PresignGet(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code, got := fetch(t, http.MethodGet, valid, nil); code != http.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("a valid grant did not work: status %d, %d bytes; the assertions below would prove nothing", code, len(got))
	}

	t.Run("a tampered signature is refused", func(t *testing.T) {
		u, err := url.Parse(valid)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		signature := q.Get("X-Amz-Signature")
		if len(signature) == 0 {
			t.Fatalf("no X-Amz-Signature in the pre-signed URL")
		}
		// One hex digit, so nothing but the signature differs.
		flipped := "0"
		if strings.HasPrefix(signature, "0") {
			flipped = "1"
		}
		q.Set("X-Amz-Signature", flipped+signature[1:])
		u.RawQuery = q.Encode()
		if code, _ := fetch(t, http.MethodGet, u.String(), nil); code == http.StatusOK {
			t.Errorf("a URL with one flipped signature digit returned 200; the signature is not being verified")
		}
	})

	t.Run("the grant stops working when it expires", func(t *testing.T) {
		// One second is minio-go's floor for an expiry, so this test costs the
		// wait. The alternative — asserting on the X-Amz-Expires parameter — is
		// the unit test below, and it proves the URL SAYS one second, not that
		// anything enforces it.
		expiring, err := store.PresignGet(ctx, key, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if code, _ := fetch(t, http.MethodGet, expiring, nil); code != http.StatusOK {
			t.Fatalf("a one-second grant was already refused at status %d; the expiry assertion below would be vacuous", code)
		}
		time.Sleep(2 * time.Second)
		if code, _ := fetch(t, http.MethodGet, expiring, nil); code == http.StatusOK {
			t.Errorf("a grant one second long still returned 200 after two seconds; expiry is not enforced")
		}
	})

	t.Run("a GET grant cannot PUT", func(t *testing.T) {
		// The fourth mutation from m6/report-object-storage.md §3, the one where
		// the stand-in did not merely answer 200 but actually overwrote the
		// object. Status is half the assertion; the bytes are the other half.
		overwrite := []byte("written with a ticket that only authorizes reading")
		code, _ := fetch(t, http.MethodPut, valid, overwrite)
		if code == http.StatusOK {
			t.Errorf("a GET grant accepted a PUT (status 200); the signature does not bind the method")
		}
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("the object was overwritten through a read-only grant: %q", got)
		}
	})
}

// The other half of 02:PORT-009, deliberately not a substitute for the one
// above: what the URL says, with no server anywhere. This half never skips, so
// a change that breaks the shape of a grant is red on every machine; it just
// cannot tell you whether anyone enforces what the URL says.
func TestPresignedURLStatesItsExpiryAndBindsItsMethod(t *testing.T) {
	t.Parallel()
	// Not New(): minio-go resolves the bucket's region before it will sign
	// anything, and against an endpoint that does not exist that is a DNS
	// failure rather than a URL. Pinning the region is the only difference, and
	// PresignGet/PresignPut below are the real methods.
	mc, err := minio.New("objstore.invalid:8333", &minio.Options{
		Creds:  credentials.NewStaticV4(devAccessKey, devSecretKey, ""),
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &Client{mc: mc, bucket: "granted"}
	ctx := t.Context()

	raw, err := store.PresignGet(ctx, "datasets/a.zip", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for _, want := range []struct{ name, field, got, expect string }{
		{"scheme", "", u.Scheme, "http"},
		{"host", "", u.Host, "objstore.invalid:8333"},
		{"one bucket, one key", "", u.Path, "/granted/datasets/a.zip"},
		{"the expiry is in the URL", "X-Amz-Expires", q.Get("X-Amz-Expires"), "900"},
		{"signed, not opaque", "X-Amz-Algorithm", q.Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256"},
	} {
		if want.got != want.expect {
			t.Errorf("%s: got %q, want %q", want.name, want.got, want.expect)
		}
	}
	if credential := q.Get("X-Amz-Credential"); !strings.HasPrefix(credential, devAccessKey+"/") {
		t.Errorf("X-Amz-Credential does not name the signing key: %q", credential)
	}

	// Method binding, without a server: the same object, the same expiry, a
	// different verb, and the signature must differ. This is the string-level
	// statement of the "a GET grant cannot PUT" property asserted for real above.
	put, err := store.PresignPut(ctx, "datasets/a.zip", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	putQuery, err := url.ParseQuery(strings.TrimPrefix(put[strings.Index(put, "?"):], "?"))
	if err != nil {
		t.Fatal(err)
	}
	if putQuery.Get("X-Amz-Signature") == q.Get("X-Amz-Signature") {
		t.Error("the GET and PUT grants for the same object share a signature, so the verb is not signed")
	}
}
