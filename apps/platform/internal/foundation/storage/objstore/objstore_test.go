package objstore

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadCappedRefusesRatherThanTruncates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		max     int
		wantErr bool
	}{
		{name: "under the ceiling", size: 3, max: 8},
		{name: "exactly at the ceiling", size: 8, max: 8},
		{name: "one byte over", size: 9, max: 8, wantErr: true},
		{name: "far over", size: 800, max: 8, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := bytes.Repeat([]byte{0x7f}, tc.size)
			got, err := readCapped(bytes.NewReader(want), tc.max, -1)
			if tc.wantErr {
				if err == nil {

					t.Fatalf("readCapped returned %d bytes and no error for an object over the ceiling", len(got))
				}
				if !strings.Contains(err.Error(), "ceiling") {
					t.Errorf("error %q does not say a limit was hit", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("readCapped: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("readCapped returned %d bytes, want the whole %d", len(got), tc.size)
			}
		})
	}
}

func TestReadCappedTrustsTheBytesNotTheAnnouncedSize(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		hint    int64
		wantErr bool
	}{
		{name: "announced exactly", size: 8, hint: 8},
		{name: "announced smaller", size: 8, hint: 2},
		{name: "announced larger", size: 3, hint: 8},
		{name: "announced far past the ceiling", size: 3, hint: 1 << 40},
		{name: "announced small but over the ceiling", size: 9, hint: 2, wantErr: true},
	} {
		want := bytes.Repeat([]byte{0x7f}, tc.size)
		got, err := readCapped(bytes.NewReader(want), 8, tc.hint)
		if tc.wantErr != (err != nil) || (!tc.wantErr && !bytes.Equal(got, want)) {
			t.Errorf("%s: got %d bytes, err %v; want the %d bytes read, refused only past the ceiling", tc.name, len(got), err, tc.size)
		}
	}
}

func TestAnAnnouncedSizeIsReadIntoOneAllocation(t *testing.T) {
	data := bytes.Repeat([]byte{0x7f}, 1<<20)
	allocs := testing.AllocsPerRun(5, func() {
		if _, err := readCapped(bytes.NewReader(data), MaxObjectBytes, int64(len(data))); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 3 {
		t.Errorf("reading a 1 MiB object of announced size took %v allocations, want it read into one buffer", allocs)
	}
}

func TestAStoreThatKeepsFailingIsTriedThreeTimesAndNoMore(t *testing.T) {
	var attempts atomic.Int32
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("location") {
			_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
			return
		}
		if r.URL.Path == "/bucket/key" {
			attempts.Add(1)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(store.Close)
	c, err := New(strings.TrimPrefix(store.URL, "http://"), "key", "secret", "bucket", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Put(context.Background(), "key", []byte("x")); err == nil {
		t.Fatal("a store answering 503 accepted the write")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("the write was attempted %d times, want 3", got)
	}
}

func bucketRaceStore(t *testing.T, existsAfterRefusal bool) *Client {
	t.Helper()
	var refused atomic.Bool
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Has("location"):
			_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`))
		case r.Method == http.MethodHead && refused.Load() && existsAfterRefusal:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut:
			refused.Store(true)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`<Error><Code>BucketAlreadyOwnedByYou</Code><Message>Your previous request to create the named bucket succeeded and you already own it.</Message></Error>`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(store.Close)
	c, err := New(strings.TrimPrefix(store.URL, "http://"), "key", "secret", "bucket", false)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestABucketAnotherProcessCreatedFirstIsReady(t *testing.T) {
	if err := bucketRaceStore(t, true).EnsureBucket(context.Background()); err != nil {
		t.Fatalf("losing the create race to another process stopped startup: %v", err)
	}
}

func TestARefusedCreateOfABucketThatStillDoesNotExistFails(t *testing.T) {
	err := bucketRaceStore(t, false).EnsureBucket(context.Background())
	if err == nil || !strings.Contains(err.Error(), "make bucket") {
		t.Fatalf("EnsureBucket = %v, want the make bucket refusal", err)
	}
}

func TestAStoreThatStopsAnsweringIsGivenUpOnAfterTwentySeconds(t *testing.T) {
	transport, err := storeTransport(false)
	if err != nil {
		t.Fatal(err)
	}
	if transport.ResponseHeaderTimeout != 20*time.Second {
		t.Errorf("response header wait = %v, want 20s", transport.ResponseHeaderTimeout)
	}
	if transport.MaxIdleConnsPerHost != 64 {
		t.Errorf("idle connections kept per host = %d, want 64", transport.MaxIdleConnsPerHost)
	}
}
