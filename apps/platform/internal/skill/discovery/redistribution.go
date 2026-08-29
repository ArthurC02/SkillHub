package catalog

// The operator surface for the redistribution verdict (02:SEC-007, 0027 extended
// by 0036). `04` 乙-17 / `05` R-3c.
//
// WHY THIS EXISTS. Two columns decide whether the platform will hand a skill's
// bytes to somebody: access_restriction (a reviewer's temporary hold) and
// redistribution (whether the licence permits copying at all). They gate the
// same download. Until now the first had a route, an operator check and an audit
// event, and the second had none of it — changing it meant running UPDATE by
// hand, leaving no record of who decided or why.
//
// The weaker reason was the governed one. That is the asymmetry the route closed
// on 2026-08-23, before either of the two questions beside it had an answer.
//
// BOTH OF THOSE ARE NOW ANSWERED (2026-08-27, `05` R-3a and R-3b, ADR-057).
//
// R-3a — who may call this — is settled as operator-only, which is what the route
// already did while waiting. The reason is not caution: ADR-021 §5.3 records that
// two repositories carried a valid MIT `LICENSE` covering content that was not
// theirs, so "the repo root says MIT" was wrong in the releasing direction. A
// judgement that people who audited it got wrong does not become more accurate
// when it is handed to whoever happens to own the skill.
//
// R-3b — what a release must carry — is settled as evidence rather than a
// confirmation box, and that is the part with teeth here: moving a skill to
// `allowed` must name the licence expression and the ADR-021 provenance tier it
// relied on, and this refuses the write when the version's frozen snapshot
// records something else, or records nothing at all. An operator who cannot name
// what the importer read has not looked at the bytes a download would hand over.
//
// It stays a claim about the newest version rather than a per-version verdict,
// because the column is on the skill. What the check buys is that the claim can
// be contradicted, and that the tier relied on lands in the audit event — so
// "every skill released on repo-license-file evidence", the exact shape of the
// §5.3 mistake, is one SQL query rather than a manual trawl.
//
// Lives in this package for the reason the file next door gives: the sentences a
// reader sees for each value are here (trust.go, redistributionDisplays), and a
// value written through a path that cannot render it is a verdict nobody can
// explain.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// operatorSettableRedistribution is the subset of the column's four values an
// operator may assert.
//
// `self_supplied` and `generated` are absent, and their absence is the
// interesting part: both are facts about where the bytes came from, established
// at the moment they arrived — the import path for one (0036), the generation
// path for the other (0037). Neither is a verdict about a licence, so there is
// nobody who could be right in asserting one after the fact: an operator who
// typed it would be claiming the platform received or wrote bytes it did not.
// The column's CHECK still permits both, because those two paths must write
// them; this route does not.
//
// Setting a self_supplied skill to `blocked` is allowed, and that direction is
// deliberate: content the owner uploaded can still turn out to be something the
// platform must stop handing back.
//
// A `generated` one is not, and that is the correction 稽核 01 forced. This
// column answers two questions at once — may these bytes be handed on, and is
// this a generated skill — because GEN-007's search exclusion has no other key
// to read (search.sql's `sk.redistribution <> 'generated'`, and the same
// predicate on the enrichment worklist). So overwriting `generated` with
// `blocked` does not tighten anything: it erases the only record that this skill
// is generated, and the skill reappears in its workspace's search and re-enters
// the paid enrichment queue. An operator pressing 「不可再散布」 would have
// published it. The refusal is in SetRedistribution below; the mechanism that
// actually holds a generated skill is access_restriction, which blocks reads,
// runs and packaging without touching provenance.
var operatorSettableRedistribution = map[string]struct{}{
	string(RedistributionAllowed): {},
	string(RedistributionBlocked): {},
	string(RedistributionUnknown): {},
}

// provenanceRedistribution is the other half of that list: values the column
// accepts and this route refuses on purpose. Named rather than left to the
// generic "unknown value" branch, because the two refusals are not the same
// answer — one says you spelled it wrong, the other says the thing you asked
// for is not a thing anyone can assert. A sixth value added to the column will
// land in the generic branch and be refused, which is the safe direction; it
// just deserves its own sentence when somebody gets round to it.
var provenanceRedistribution = map[string]string{
	string(RedistributionSelfSupplied): "self_supplied is not a verdict anyone can assert: it records that this " +
		"workspace supplied the bytes, and only the import path can establish that",
	string(RedistributionGenerated): "generated is not a verdict anyone can assert: it records that the platform " +
		"wrote these bytes for this workspace, and only the generation path can establish that",
}

// redistributionRequest is the body of PUT /admin/skills/{id}/redistribution.
//
// `value` and `note` are always required: an operator action nobody can explain
// later is not a decision (02:SEC-011's rule for the neighbouring route, applied
// here for the same reason).
//
// The two licence fields are required only for `allowed`, and only there because
// that is the only value that releases anything. Demanding evidence in order to
// block, or to un-decide, would be asking for a licensing judgement as the price
// of refusing to make one — and those are the directions ADR-021 §5.3 says a
// mistake is allowed to fall in.
type redistributionRequest struct {
	Value             string `json:"value"`
	Note              string `json:"note"`
	LicenseExpression string `json:"license_expression"`
	LicenseSource     string `json:"license_source"`
}

// LicenseClaim is what an operator asserts they read before releasing a skill:
// the SPDX expression and the ADR-021 tier it came from, together, because
// ADR-021 決策 1 is that the two are one claim — and flattening them into a
// single string is how a repository's MIT came to be read as a subdirectory's.
type LicenseClaim struct {
	Expression string
	Source     string
}

// SetRedistribution handles PUT /admin/skills/{id}/redistribution.
//
// Idempotent: writing the value a skill already has is a second audit event and
// no change to the row. Same choice as the restriction routes — an operator
// repeating an action is not an error, and a 409 would only invite retry loops.
func (h *Handler) SetRedistribution(w http.ResponseWriter, r *http.Request) {
	var body redistributionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a value and a note")
		return
	}

	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}

	previous, err := h.Svc.SetRedistribution(r.Context(), skillID, user.ID, body.Value, body.Note,
		LicenseClaim{Expression: body.LicenseExpression, Source: body.LicenseSource})
	var inputErr restrictionInputError
	if errors.As(err, &inputErr) {
		httpx.WriteError(w, http.StatusBadRequest, inputErr.Error())
		return
	}
	if errors.Is(err, errSkillNotFound) {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "redistribution change failed")
		return
	}

	value := Redistribution(strings.TrimSpace(body.Value))
	display := value.Display()
	// The response echoes the sentence the public detail view will now show, so
	// the operator can see what a reader will see without fetching the skill.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id": pgconv.UUIDString(skillID),
		"redistribution": map[string]any{
			"value": string(value),
			"label": display.Label,
			"note":  display.Note,
		},
		"previous_value": previous,
	})
}

// SetRedistribution validates the verdict and the explanation, then writes both
// the column and the event that accounts for it. Every invariant is here rather
// than in the handler so a non-HTTP caller cannot skip one.
func (s *Service) SetRedistribution(
	ctx context.Context, skillID, actor pgtype.UUID, value, note string, claim LicenseClaim,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", restrictionInputError("value is required")
	}
	if _, ok := operatorSettableRedistribution[value]; !ok {
		// The provenance values are named in the message rather than lumped in
		// with typos: a caller who tried one made a category error, not a
		// spelling mistake, and telling them the list of valid values would not
		// explain why the one they picked is missing from it.
		if msg, ok := provenanceRedistribution[value]; ok {
			return "", restrictionInputError(msg)
		}
		return "", restrictionInputError(
			"unknown redistribution value; settable values: allowed, blocked, unknown")
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return "", err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The evidence check runs inside the transaction that writes, so a version
	// landing in between cannot release a licence nobody read.
	var verified LicenseClaim
	if value == string(RedistributionAllowed) {
		verified, err = checkLicenseEvidence(ctx, tx, skillID, claim)
		if err != nil {
			return "", err
		}
	}

	before, err := registry.SetRedistribution(ctx, tx, skillID, value)
	if errors.Is(err, registry.ErrNotFound) {
		return "", errSkillNotFound
	}
	if err != nil {
		return "", err
	}
	// Checked after the write and unwound by the deferred rollback, because the
	// current value is what the write returns and reading it first would be a
	// second read of a row this transaction is about to take a lock on anyway.
	//
	// Nothing lands: the audit event below never runs, so there is no record of a
	// change that did not happen either.
	if before.Redistribution == string(RedistributionGenerated) {
		return "", restrictionInputError(
			"this skill was generated by the platform, and `generated` is the only record of that. " +
				"Overwriting it would put the skill back into its workspace's search results and into " +
				"the enrichment queue (GEN-007 keys on this column). To stop it being read, run or " +
				"packaged, set access_restriction instead — that holds the content without erasing " +
				"where it came from")
	}
	// One transaction with the write above. This gate decides whether content
	// leaves the platform, so "released, and no record of who released it" is
	// the one outcome that must be impossible (iron rule 9).
	//
	// workspace_id is the affected skill's, which is how a cross-workspace
	// operator action stays reviewable per workspace. Recording it is not a
	// scope violation: nothing about that workspace's contents was read.
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillRedistribution,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata:     redistributionMetadata(before.Redistribution, value, note, verified),
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return before.Redistribution, nil
}

// checkLicenseEvidence refuses a release the frozen snapshot does not support
// (`05` R-3b, ADR-057).
//
// Three refusals, and they are three different sentences on purpose. Nothing
// named at all is an operator who skipped a field; nothing recorded is a skill
// with no evidence to rely on, which no amount of typing will fix; a mismatch is
// an operator describing a package other than this one. Collapsing them into
// "invalid licence evidence" would leave the middle case reading like a form
// error, and it is the one that must not be worked around.
//
// Comparison is trimmed and case-insensitive. SPDX identifiers are defined
// case-insensitively, and refusing a release over `mit` against `MIT` would teach
// operators to paste rather than read — the opposite of what the rule is for.
// It returns the snapshot's own values, not the operator's, and the difference
// is not cosmetic: the two are equal here only up to case and whitespace, and the
// query this feeds — which releases leaned on which tier — has to match a tier
// name exactly. A trail recording `MANIFEST` because that is what somebody typed
// is a row that audit misses. (It did, on the first run of the test below.)
func checkLicenseEvidence(
	ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, claim LicenseClaim,
) (LicenseClaim, error) {
	claim.Expression = strings.TrimSpace(claim.Expression)
	claim.Source = strings.TrimSpace(claim.Source)
	if claim.Expression == "" || claim.Source == "" {
		return LicenseClaim{}, restrictionInputError(
			"releasing a skill requires the licence evidence it relies on: license_expression and " +
				"license_source, as recorded on the skill's newest version (05 R-3b: a confirmation " +
				"box is not evidence)")
	}

	expression, source, err := registry.LicenseEvidence(ctx, tx, skillID)
	if errors.Is(err, registry.ErrNotFound) {
		return LicenseClaim{}, errSkillNotFound
	}
	if err != nil {
		return LicenseClaim{}, err
	}
	if expression == "" || source == "" {
		return LicenseClaim{}, restrictionInputError(
			"this skill's newest version records no licence, so there is no evidence to rely on; " +
				"re-import it if the package carries one (ADR-021 §5: writing a tier onto an existing " +
				"row would be inventing the evidence rather than reading it)")
	}
	if !strings.EqualFold(expression, claim.Expression) || !strings.EqualFold(source, claim.Source) {
		return LicenseClaim{}, restrictionInputError(fmt.Sprintf(
			"licence evidence does not match what this skill's newest version records (recorded: "+
				"%s from %s); releasing it would assert a licence the platform never read",
			expression, source))
	}
	return LicenseClaim{Expression: expression, Source: source}, nil
}

// redistributionMetadata records the evidence beside the verdict, and only beside
// the verdict that used it.
//
// The two fields are absent rather than empty for `blocked` and `unknown`,
// because an empty licence beside a block would read as "released on no evidence"
// to whoever queries this table later — and the query this shape exists to serve
// is exactly that one: which releases leaned on which tier (ADR-021 §5.3).
func redistributionMetadata(before, after, note string, verified LicenseClaim) map[string]any {
	m := map[string]any{"before": before, "after": after, "note": note}
	if after == string(RedistributionAllowed) {
		m["license_expression"] = verified.Expression
		m["license_source"] = verified.Source
	}
	return m
}
