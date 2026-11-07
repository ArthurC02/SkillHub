package run

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

func repoFileBounds(t *testing.T, path []string, pattern string) (lo, hi int64) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "..", "..", ".."}, path...)...))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(pattern).FindAllSubmatch(body, -1)
	if len(found) != 1 {
		t.Fatalf("%v: %d matches for %q, want exactly one", path, len(found), pattern)
	}
	lo, errLo := strconv.ParseInt(string(found[0][1]), 10, 64)
	hi, errHi := strconv.ParseInt(string(found[0][2]), 10, 64)
	if errLo != nil || errHi != nil {
		t.Fatal(errLo, errHi)
	}
	return lo, hi
}

func TestTheRunTokensPerMinuteRangeIsTheSameInGoTheContractAndTheTable(t *testing.T) {
	want := [2]int64{MinRunTPMLimit, MaxRunTPMLimit}
	sqlLo, sqlHi := repoFileBounds(t, []string{"db", "migrations", "0138_run_rate_limit_setting.sql"},
		`tpm_limit BETWEEN (\d+) AND (\d+)`)
	apiLo, apiHi := repoFileBounds(t, []string{"contracts", "openapi", "public.yaml"},
		`(?s)operationId: setRunTokensPerMinute\b.*?limit:\s+type: integer\s+minimum: (\d+)\s+maximum: (\d+)`)
	for _, tc := range []struct {
		name string
		got  [2]int64
	}{
		{"the table check", [2]int64{sqlLo, sqlHi}},
		{"the contract", [2]int64{apiLo, apiHi}},
		{"the generated contract maximum", [2]int64{MinRunTPMLimit, limits.SetRunTokensPerMinuteBodyLimitMaximum}},
	} {
		if tc.got != want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, want)
		}
	}
}

func TestTheRunBudgetRangeIsTheSameInGoAndTheTable(t *testing.T) {
	lo, hi := repoFileBounds(t, []string{"db", "migrations", "0137_run_model_budget_setting.sql"},
		`budget_micros BETWEEN (\d+) AND (\d+)`)
	want := [2]int64{int64(MinRunBudgetUSD * microsPerUSD), int64(MaxRunBudgetUSD * microsPerUSD)}
	if got := [2]int64{lo, hi}; got != want {
		t.Errorf("the table check = %v micros, want %v from the Go dollars", got, want)
	}
}
