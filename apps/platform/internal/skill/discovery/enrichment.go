package catalog

type EnrichmentStatus string

const (
	EnrichmentPending  EnrichmentStatus = "pending"
	EnrichmentEnriched EnrichmentStatus = "enriched"
)

func AllEnrichmentStatuses() []EnrichmentStatus {
	return []EnrichmentStatus{EnrichmentPending, EnrichmentEnriched}
}
