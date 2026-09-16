package catalog

type EnrichmentStatus string

const (
	EnrichmentPending  EnrichmentStatus = "pending"
	EnrichmentEnriched EnrichmentStatus = "enriched"
)

func (s EnrichmentStatus) restartsAttempts() bool { return s == EnrichmentPending }

func (s EnrichmentStatus) listable(hasEmbedding bool) bool {
	return s == EnrichmentEnriched || hasEmbedding
}

func AllEnrichmentStatuses() []EnrichmentStatus {
	return []EnrichmentStatus{EnrichmentPending, EnrichmentEnriched}
}
