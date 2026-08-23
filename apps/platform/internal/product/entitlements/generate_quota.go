package policy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

// The generation allowance (GEN-004, ADR-047 決策 5).
//
// A second allowance rather than a second draw on the first one. Sharing one
// pool would let a generation quietly eat a trial run's balance — and the trial
// run is the thing the MVP funnel is measuring. Letting a path from outside the
// MVP erode the MVP's own measurement mixes two experiments (ADR-047 決策 5).
//
// It reuses QuotaLimits and Usage deliberately. The shape of an allowance —
// daily ceiling, rolling window, smaller first window — is the same question
// asked about a different noun, and two copies of that arithmetic is how the two
// stop agreeing about when the window resets.
//
// What it does NOT reuse is the counting query. Runs are counted by their
// `preparing` transition; generations are counted by the skill_sources rows they
// produced, which is what makes two of ADR-047 決策 2's rules free rather than
// implemented: a retry writes no second row, so the unit is one generation and
// not one gateway call, and a failed generation writes no row at all, so it
// costs nothing. There is nothing to decrement and therefore nothing that can
// decrement wrongly.

// ErrGenerateQuotaExceeded is the generation refusal. Separate from
// ErrQuotaExceeded so that a caller cannot accidentally treat one allowance's
// exhaustion as the other's — and so the sentence a user reads names the thing
// they actually ran out of.
var ErrGenerateQuotaExceeded = errors.New("this workspace has used its free generation allowance")

// 待追認 — every one of them.
//
// PDM-010 §8.1 gives numbers for runs and says nothing about generation, because
// generation did not exist when it was written. ADR-028 決策 4 permits exactly
// this: the enforcement point may be built before the numbers are ratified,
// because it does not depend on them. What may not happen before ratification is
// putting an unratified number on a screen — so until PDM signs these, a
// deployment that shows a generation allowance is showing a guess (04 乙-22).
//
// Where the guesses come from, so that ratifying them is a review and not a
// fresh start: a generation measured $0.00553 with the default mini model and
// $0.1186 with the flagship (m5/report-generate-baseline.md). That is cheaper
// than the $0.0382 median gateway spend of a Run, and a user who does not like
// what came back will rewrite the task description and go again — so the daily
// number is higher than the Run's five, and the window number is not.
const (
	generateDaily       = 10 // per rolling 24 hours — 待追認
	generateWindow      = 30 // per rolling window thereafter — 待追認
	generateFirstWindow = 20 // a workspace's first window — 待追認
	generateWindowDays  = 30 // the rolling window's length — 待追認
)

// DefaultGenerateQuotaLimits is the unratified proposal above. The zero value —
// what a deployment gets by turning the allowance off — enforces nothing and
// displays nothing, exactly as QuotaLimits.Enforced already defines.
func DefaultGenerateQuotaLimits() QuotaLimits {
	return QuotaLimits{
		Daily:       generateDaily,
		Window:      generateWindow,
		FirstWindow: generateFirstWindow,
		WindowDays:  generateWindowDays,
	}
}

// EnforceGenerateQuota refuses a generation the workspace has no allowance for.
//
// It is called BEFORE the model call, which is the whole point: 02:GEN-001 says
// an exhausted allowance is refused 不得先花錢再說. That also means it cannot be
// inside the transaction that writes the result the way EnforceQuota is — the
// model call sits in between, and holding a transaction open across it would
// hold a connection for the length of a generation. The overshoot that allows is
// stated rather than implied: concurrent generations in one workspace all read
// the same usage before any of them writes a row.
//
// What bounds it is not here — ingest holds one generation slot per workspace,
// so within one API replica the concurrent readers are at most one. That gate is
// in-process, which is the ceiling: two replicas, two slots, two readers.
//
// ponytail: overshoot bounded by replica count. Add a per-workspace advisory
// lock around the count when there is more than one cmd/api.
func EnforceGenerateQuota(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID,
) (string, error) {
	return enforce(ctx, reader, l, workspaceID, allowance{
		sentinel: ErrGenerateQuotaExceeded, noun: "generations", prefix: "generate_quota",
	})
}
