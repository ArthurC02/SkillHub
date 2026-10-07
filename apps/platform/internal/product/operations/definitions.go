package operations

const dailyReportSpendCapMicros = 200_000

const ToolMaintenanceReport = "maintenance_report"

var DailyReport = Definition{
	Name:                "daily-report",
	Purpose:             "Reads the daily maintenance report and tells operators, in plain words, what is fine and what needs attention.",
	ModelRole:           "skillhub-ops-report",
	DailySpendCapMicros: dailyReportSpendCapMicros,
	Tools:               []string{ToolMaintenanceReport},
	CheckResult:         CitesOnlyReturnedFacts,
	Sightings:           DailyReportSightings,
}

func Definitions() []Definition {
	return []Definition{DailyReport}
}

func Lookup(name string) (Definition, bool) {
	for _, def := range Definitions() {
		if def.Name == name {
			return def, true
		}
	}
	return Definition{}, false
}
