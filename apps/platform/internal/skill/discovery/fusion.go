package catalog

import (
	"bytes"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type hybridCandidate struct {
	ranked   bool
	distance float64
	covered  bool
}

func fuseHybridCandidates(rows []gen.ListHybridSearchCandidatesRow) (map[pgtype.UUID]hybridCandidate, []pgtype.UUID) {
	fused := map[pgtype.UUID]hybridCandidate{}
	var order []pgtype.UUID
	for _, row := range rows {
		c, seen := fused[row.SkillID]
		if !seen {
			order = append(order, row.SkillID)
		}
		c.covered = c.covered || row.Lexical
		if !row.Unembedded && (!c.ranked || row.Distance < c.distance) {
			c.ranked, c.distance = true, row.Distance
		}
		fused[row.SkillID] = c
	}
	return fused, order
}

func (c hybridCandidate) admittedWithin(maxDistance float64) bool {
	return c.covered || !c.ranked || c.distance <= maxDistance
}

func admittedCandidates(fused map[pgtype.UUID]hybridCandidate, order []pgtype.UUID, maxDistance float64) []pgtype.UUID {
	var admitted []pgtype.UUID
	for _, id := range order {
		if fused[id].admittedWithin(maxDistance) {
			admitted = append(admitted, id)
		}
	}
	return admitted
}

func firstPage[T any](rows []T, limit int32) ([]T, int64) {
	return rows[:min(len(rows), int(limit))], int64(len(rows))
}

func rankHybridDocuments(docs []gen.ListHybridSearchDocumentsRow, fused map[pgtype.UUID]hybridCandidate, query string) {
	exact := strings.ToLower(strings.Trim(query, " "))
	sort.SliceStable(docs, func(i, j int) bool {
		a, b := fused[docs[i].SkillID], fused[docs[j].SkillID]
		aExact, bExact := strings.ToLower(docs[i].Name) == exact, strings.ToLower(docs[j].Name) == exact
		switch {
		case aExact != bExact:
			return aExact
		case a.covered != b.covered:
			return a.covered
		case a.ranked != b.ranked:
			return a.ranked
		case a.distance != b.distance:
			return a.distance < b.distance
		}
		return bytes.Compare(docs[i].SkillID.Bytes[:], docs[j].SkillID.Bytes[:]) < 0
	})
}
