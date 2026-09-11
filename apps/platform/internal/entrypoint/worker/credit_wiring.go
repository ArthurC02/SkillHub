package worker

import (
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

var creditStatKinds = []string{
	credit.KindCreationStep,
	credit.KindSearchEmbedding,
	credit.KindIndexEnrich,
	credit.KindReview,
	credit.KindSuggestion,
	credit.KindGenerate,
	credit.KindMatchReasons,
	credit.KindRun,
	credit.KindCreationSession,
}

const creditStatWindow = 7 * 24 * time.Hour

func wireCostRecording(svc *credit.Service, search *catalog.Service, versions, backfill *ingest.Service, evaluations *eval.Service) {
	search.Credit = svc
	versions.Credit = svc
	if backfill != nil {
		backfill.Credit = svc
	}
	evaluations.Credit = svc
}
