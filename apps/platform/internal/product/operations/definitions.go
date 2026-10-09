package operations

const dailyReportSpendCapMicros = 200_000

const ToolMaintenanceReport = "maintenance_report"

var DailyReport = Definition{
	Name:                "daily-report",
	Purpose:             "Reads the daily maintenance report and tells operators, in plain words, what is fine and what needs attention.",
	ModelRole:           "skillhub-ops-report",
	DailySpendCapMicros: dailyReportSpendCapMicros,
	Tools:               []string{ToolMaintenanceReport},
	Actions:             maintenanceJobActions(),
	CheckResult:         CitesOnlyReturnedFacts,
	Sightings:           DailyReportSightings,
	Proposals:           DailyReportProposals,
}

const exposureReviewSpendCapMicros = 200_000

const ToolExposureQueue = "exposure_queue"

var ExposureReview = Definition{
	Name:                "exposure-review",
	Purpose:             "Reads the publications waiting for an exposure decision and points operators at the ones whose search text or scan findings need a closer look; the decision stays with the operator.",
	ModelRole:           "skillhub-ops-report",
	DailySpendCapMicros: exposureReviewSpendCapMicros,
	Tools:               []string{ToolExposureQueue},
	Actions:             []string{},
	CheckResult:         CitesOnlyReturnedFacts,
	Sightings:           DailyReportSightings,
}

func maintenanceJobActions() []string {
	actions := make([]string, len(ProposableMaintenanceJobs))
	for i, job := range ProposableMaintenanceJobs {
		actions[i] = MaintenanceJobAction(job)
	}
	return actions
}

func Definitions() []Definition {
	return []Definition{DailyReport, ExposureReview}
}

func Lookup(name string) (Definition, bool) {
	for _, def := range Definitions() {
		if def.Name == name {
			return def, true
		}
	}
	return Definition{}, false
}
