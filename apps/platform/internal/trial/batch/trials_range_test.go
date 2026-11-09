package batch

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestTheBatchTrialsRangeIsTheSameInGoTheContractAndTheTable(t *testing.T) {
	sources := []struct {
		name    string
		path    []string
		pattern string
	}{
		{"the table check", []string{"db", "migrations", "0118_evaluation_batches.sql"},
			`trials\s+integer NOT NULL CHECK \(trials BETWEEN (\d+) AND (\d+)\)`},
		{"the contract", []string{"contracts", "openapi", "public.yaml"},
			`(?s)operationId: createEvaluationBatch\b.*?trials: \{ type: integer, minimum: (\d+), maximum: (\d+) \}`},
	}
	want := [2]int64{MinBatchTrials, MaxBatchTrials}
	for _, src := range sources {
		body, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "..", "..", ".."}, src.path...)...))
		if err != nil {
			t.Fatal(err)
		}
		found := regexp.MustCompile(src.pattern).FindAllSubmatch(body, -1)
		if len(found) != 1 {
			t.Fatalf("%s: %d matches, want exactly one", src.name, len(found))
		}
		lo, errLo := strconv.ParseInt(string(found[0][1]), 10, 64)
		hi, errHi := strconv.ParseInt(string(found[0][2]), 10, 64)
		if errLo != nil || errHi != nil {
			t.Fatal(errLo, errHi)
		}
		if got := [2]int64{lo, hi}; got != want {
			t.Errorf("%s = %v, want %v from Go", src.name, got, want)
		}
	}
}
