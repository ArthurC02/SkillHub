package sandbox

import (
	"reflect"
	"testing"
)

func TestDatasetNamesSkipsEmptyKeysAndUnsafeNames(t *testing.T) {
	req := RunRequest{TestCase: TestCaseSnapshotRef{DatasetRefs: []DatasetRef{
		{ObjectKey: "k1", FileName: "input.csv"},
		{ObjectKey: "k4", FileName: `nested\dir\data.csv`},
		{ObjectKey: "", FileName: "no-key.csv"},
		{ObjectKey: "k2", FileName: ".."},
		{ObjectKey: "k3", FileName: "-rf"},
	}}}

	got := req.DatasetNames()
	want := map[string]string{"k1": "input.csv", "k4": "data.csv"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DatasetNames = %v, want %v", got, want)
	}
}
