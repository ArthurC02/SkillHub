package objstore

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

	devAccessKey = "skillhubdev"
	devSecretKey = "skillhubdevsecret"

	testBucket = "skillhub-presign-test"
)

var presignStore *Client

func TestMain(m *testing.M) {
	endpoint := os.Getenv(objstoreEndpointEnv)
	if endpoint == "" {

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

func TestPresignedURLStatesItsExpiryAndBindsItsMethod(t *testing.T) {
	t.Parallel()

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
