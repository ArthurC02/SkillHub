package testlab

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type countingReader struct {
	io.Reader
	read int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

type datasetBodyStore struct {
	recordingStore
	bodies map[string]*countingReader
}

func (s *datasetBodyStore) Open(_ context.Context, key string) (io.ReadCloser, int64, error) {
	body, ok := s.bodies[key]
	if !ok {
		return nil, 0, errors.New("no such object")
	}
	return io.NopCloser(body), 0, nil
}

func TestADatasetOutlineReadsOnlyTheHeadOfALargeFile(t *testing.T) {
	csv := "id,total\n" + strings.Repeat("1,2.5\n", 4*datasetHeadBytes/6)
	body := &countingReader{Reader: strings.NewReader(csv)}
	svc := &Service{Store: &datasetBodyStore{bodies: map[string]*countingReader{"rows": body}}}

	outlines := svc.outlineDatasets(t.Context(), []gen.Dataset{
		{FileName: "rows.csv", ContentType: "text/csv", ObjectKey: "rows"},
		{FileName: "gone.csv", ContentType: "text/csv", ObjectKey: "gone"},
	})

	if len(outlines) != 2 {
		t.Fatalf("got %d outlines, want 2", len(outlines))
	}
	fields := outlines[0].Fields
	if len(fields) != 2 || fields[0].Name != "id" || fields[1].Name != "total" || fields[1].InferredType != "number" {
		t.Errorf("fields = %+v, want id and a numeric total", fields)
	}
	if body.read > datasetHeadBytes {
		t.Errorf("read %d bytes of a %d-byte file, want at most the %d-byte head", body.read, len(csv), datasetHeadBytes)
	}
	if outlines[1].Fields != nil || outlines[1].FileName != "gone.csv" {
		t.Errorf("an unreadable dataset = %+v, want its name without fields", outlines[1])
	}
}
