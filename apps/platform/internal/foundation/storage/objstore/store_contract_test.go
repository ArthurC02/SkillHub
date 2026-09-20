package objstore

import (
	"bytes"
	"context"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore/objstoretest"
)

type storeUnderTest interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	GetIfPresent(ctx context.Context, key string) ([]byte, bool, error)
	Exists(ctx context.Context, key string) (bool, error)
	Remove(ctx context.Context, key string) error
}

func storesUnderTest(t *testing.T) map[string]storeUnderTest {
	t.Helper()
	inProcess, stop, err := NewInProcess("contract-bucket")
	if err != nil {
		t.Fatalf("NewInProcess: %v", err)
	}
	t.Cleanup(stop)

	stores := map[string]storeUnderTest{
		"in-memory fake": objstoretest.New(),
		"in-process":     inProcess,
	}
	if presignStore != nil {
		stores["the configured store"] = presignStore
	}
	return stores
}

func eachStore(t *testing.T, assert func(t *testing.T, store storeUnderTest, key string)) {
	t.Helper()
	for name, store := range storesUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			assert(t, store, "contract/"+t.Name())
		})
	}
}

func TestEveryStoreReportsAnAbsentObjectAsAbsenceNotFailure(t *testing.T) {
	eachStore(t, func(t *testing.T, store storeUnderTest, key string) {
		ctx := t.Context()

		exists, err := store.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists on a key never written = %v; the caller cannot tell a missing object "+
				"from a store it could not reach", err)
		}
		if exists {
			t.Error("Exists reported an object nobody wrote")
		}

		data, found, err := store.GetIfPresent(ctx, key)
		if err != nil {
			t.Fatalf("GetIfPresent on a key never written = %v, want absence without an error", err)
		}
		if found || len(data) != 0 {
			t.Errorf("GetIfPresent returned %q found=%v for a key nobody wrote", data, found)
		}

		if _, err := store.Get(ctx, key); err == nil {
			t.Error("Get on a key nobody wrote succeeded; a caller that demands an object would " +
				"carry on with nothing")
		}
	})
}

func TestEveryStoreReadsBackExactlyWhatWasWritten(t *testing.T) {
	eachStore(t, func(t *testing.T, store storeUnderTest, key string) {
		ctx := t.Context()
		want := []byte("PK\x03\x04 a package\x00 with NUL and 中文")

		if err := store.Put(ctx, key, want); err != nil {
			t.Fatalf("Put: %v", err)
		}
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("Get returned %q, want %q", got, want)
		}

		got, found, err := store.GetIfPresent(ctx, key)
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Errorf("GetIfPresent = %q found=%v err=%v, want the bytes and found", got, found, err)
		}
		exists, err := store.Exists(ctx, key)
		if err != nil || !exists {
			t.Errorf("Exists = %v err=%v after a write, want true", exists, err)
		}
	})
}

func TestEveryStoreLetsAnEmptyObjectRoundTrip(t *testing.T) {
	eachStore(t, func(t *testing.T, store storeUnderTest, key string) {
		ctx := t.Context()
		if err := store.Put(ctx, key, []byte{}); err != nil {
			t.Fatalf("Put of an empty object: %v", err)
		}
		data, found, err := store.GetIfPresent(ctx, key)
		if err != nil {
			t.Fatalf("GetIfPresent: %v", err)
		}
		if !found {
			t.Error("an object written with no bytes reads back as absent; a caller cannot tell it " +
				"from one that was never written")
		}
		if len(data) != 0 {
			t.Errorf("GetIfPresent returned %q for an object written empty", data)
		}
	})
}

func TestEveryStoreReplacesWhatIsAlreadyUnderTheKey(t *testing.T) {
	eachStore(t, func(t *testing.T, store storeUnderTest, key string) {
		ctx := t.Context()
		if err := store.Put(ctx, key, []byte("first")); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := store.Put(ctx, key, []byte("second")); err != nil {
			t.Fatalf("Put over an existing key: %v", err)
		}
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(got) != "second" {
			t.Errorf("Get returned %q after a second write, want the newer bytes", got)
		}
	})
}

func TestEveryStoreForgetsWhatWasRemovedAndRepeatsSafely(t *testing.T) {
	eachStore(t, func(t *testing.T, store storeUnderTest, key string) {
		ctx := t.Context()
		if err := store.Put(ctx, key, []byte("bytes")); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := store.Remove(ctx, key); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		exists, err := store.Exists(ctx, key)
		if err != nil || exists {
			t.Errorf("Exists = %v err=%v after Remove, want false", exists, err)
		}
		if _, found, err := store.GetIfPresent(ctx, key); err != nil || found {
			t.Errorf("GetIfPresent found=%v err=%v after Remove, want absence", found, err)
		}

		if err := store.Remove(ctx, key); err != nil {
			t.Errorf("Remove of an object already gone = %v; cleanup runs more than once and must "+
				"be safe to repeat", err)
		}
		if err := store.Remove(ctx, key+"/never-written"); err != nil {
			t.Errorf("Remove of a key nobody wrote = %v; cleanup must not need to know what is there", err)
		}
	})
}

func TestPutRefusesWhatGetCouldNeverReadBack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		max     int
		wantErr bool
	}{
		{name: "under the ceiling", size: 3, max: 8},
		{name: "exactly at the ceiling", size: 8, max: 8},
		{name: "one byte over", size: 9, max: 8, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := withinObjectCeiling(tc.size, tc.max)
			if tc.wantErr && err == nil {
				t.Fatal("a write past the read ceiling was accepted; the object would be stored and " +
					"never readable")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("withinObjectCeiling(%d, %d) = %v", tc.size, tc.max, err)
			}
		})
	}
}
