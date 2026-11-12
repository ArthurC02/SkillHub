package creation

import (
	"math"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestCreationCostCoverageDistinguishesUnknownUnsettledAndKnownZero(t *testing.T) {
	for _, tc := range []struct {
		name, spent, reserved, unknown, coverage string
		cost                                     float64
	}{
		{"known zero", "0", "0", "false", "known", 0},
		{"known negative zero", "-0", "0", "false", "known", 0},
		{"known positive", "0.02", "0", "false", "known", 0.02},
		{"largest finite spend", "1.7976931348623157e308", "0", "false", "known", math.MaxFloat64},
		{"unsettled", "0.02", "0.1", "false", "unsettled", 0},
		{"unknown usage before unsettled", "0.02", "0.1", "true", "unknown", 0},
		{"missing spend", "", "0", "false", "unknown", 0},
		{"null spend", "null", "0", "false", "unknown", 0},
		{"string spend", `"0"`, "0", "false", "unknown", 0},
		{"boolean spend", "false", "0", "false", "unknown", 0},
		{"array spend", "[]", "0", "false", "unknown", 0},
		{"negative spend", "-0.01", "0", "false", "unknown", 0},
		{"out of range spend", "1e309", "0", "false", "unknown", 0},
		{"just above finite spend", "1.7976931348623158e308", "0", "false", "unknown", 0},
		{"just below zero spend", "-5e-324", "0", "false", "unknown", 0},
		{"positive spend underflow", "1e-999", "0", "false", "unknown", 0},
		{"negative spend underflow", "-1e-999", "0", "false", "unknown", 0},
		{"missing reserve", "0", "", "false", "unknown", 0},
		{"null reserve", "0", "null", "false", "unknown", 0},
		{"string reserve", "0", `"0"`, "false", "unknown", 0},
		{"negative reserve", "0", "-0.01", "false", "unknown", 0},
		{"out of range reserve", "0", "1e309", "false", "unknown", 0},
		{"largest finite reserve", "0", "1.7976931348623157e308", "false", "unsettled", 0},
		{"just above finite reserve", "0", "1.7976931348623158e308", "false", "unknown", 0},
		{"just below zero reserve", "0", "-5e-324", "false", "unknown", 0},
		{"smallest positive reserve", "0", "5e-324", "false", "unsettled", 0},
		{"positive reserve underflow", "0", "1e-999", "false", "unknown", 0},
		{"missing usage flag", "0", "0", "", "unknown", 0},
		{"null usage flag", "0", "0", "null", "unknown", 0},
		{"string usage flag", "0", "0", `"false"`, "unknown", 0},
		{"number usage flag", "0", "0", "0", "unknown", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			coverage, cost := creationCostCoverage(gen.ListCreationCostObservationsRow{
				SpentUsd: []byte(tc.spent), ReservedUsd: []byte(tc.reserved), UsageUnknown: []byte(tc.unknown),
			})
			if coverage != tc.coverage || cost != tc.cost {
				t.Fatalf("coverage=%s cost=%g; want %s %g", coverage, cost, tc.coverage, tc.cost)
			}
		})
	}
}
