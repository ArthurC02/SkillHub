package operations

const dailyReportSpendCapMicros = 200_000

var DailyReport = Definition{
	Name:                "daily-report",
	Purpose:             "Reads the daily maintenance report and tells operators, in plain words, what is fine and what needs attention.",
	ModelRole:           "skillhub-ops-report",
	DailySpendCapMicros: dailyReportSpendCapMicros,
	Tools:               []string{"maintenance_report"},
}

func Definitions() []Definition {
	return []Definition{DailyReport}
}
