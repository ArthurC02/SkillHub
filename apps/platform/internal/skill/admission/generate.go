package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"gopkg.in/yaml.v3"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// Generation from a task description (GEN-003, ADR-046).
//
// It lives in ingest and not next to the other LLM callers because the thing it
// produces is an import: the bytes go through prepare → Validate → tx
// exactly as an upload does, and a generated package that fails validation fails
// it for the same reason and with the same words. A second version-creation path
// would be a second truth about what a valid package is — the failure PACK-002
// was ruled against by name.
//
// What is specific to generation is only the two decisions above that path:
// which failures are worth a second attempt, and what the source row says.

// generateMaxAttempts is 2: the model gets one more go at the same prompt.
//
// ADR-047 決策 1 set the number at exactly one retry, and the B round then
// measured what it buys: 80% to 90%, not to nothing. 6 of 20 task descriptions
// failed at least once and 2 of 20 failed both times, so the residual is real
// and the UI is forbidden from promising a success rate (02:GEN-003).
const generateMaxAttempts = 2 // one-number: generateMaxAttempts

// ErrGenerateBlank: nothing to generate from. Refused before the gateway, so a
// blank box never costs money (02:GEN-001). apps/llm refuses it again as a
// backstop; this is the product rule.
var ErrGenerateBlank = errors.New("ingest: task description is empty or too short to act on")

// ErrGenerateTooLong: past what one generation takes as input. Refused here
// rather than at the gateway, for the same reason as the floor.
var ErrGenerateTooLong = errors.New("ingest: task description is too long")

// ErrGenerateInFlight: this workspace already has a generation running.
var ErrGenerateInFlight = errors.New("ingest: a generation is already running for this workspace")

// The same two numbers apps/llm's GenerateSkillRequest carries, in the language
// that owns product rules (iron rule 6). Runes, not bytes.
const (
	minTaskDescriptionRunes = 8
	maxTaskDescriptionRunes = 4000 // one-number: generateMaxTaskRunes
)

// ErrGenerateNotForCatalogue: generation is a personal-workspace feature
// (ADR-046 決策 1). See the check in GenerateSkill for why it is enforced rather
// than assumed.
var ErrGenerateNotForCatalogue = errors.New("ingest: the catalogue does not generate skills")

// ErrGenerateNoInput: neither a task description nor a diagram was given
// (02:GEN-005). Refused before the gateway, same reasoning as ErrGenerateBlank.
var ErrGenerateNoInput = errors.New("ingest: nothing to generate from — write a description or attach a diagram")

// ErrDiagramInvalid: the diagram is not a usable image — wrong media type,
// zero bytes, or over generateMaxDiagramBytes once decoded (02:GEN-005). The
// platform does not resize an oversized image, it refuses it (ADR-047 決策 1's
// "no editing of inputs" applied one door earlier).
var ErrDiagramInvalid = errors.New("ingest: diagram is not a usable image")

// ErrTooManyReferences: more than generateMaxReferences reference skill ids
// were given (02:GEN-006).
var ErrTooManyReferences = errors.New("ingest: too many reference skills")

// ErrReferenceUnavailable: a reference skill id could not be used as a
// worked example — not found in the caller's workspace or the catalogue, taken
// down, under a licensing hold, `redistribution = blocked`, or its SKILL.md
// could not be read back out of storage (02:GEN-006). One sentinel for every
// one of those: which of them it was is not something the caller acts on
// differently, and NFR-002/iron rule 3 both say not to describe another
// workspace's private skill by name in a refusal.
var ErrReferenceUnavailable = errors.New("ingest: a reference skill could not be used")

// generateMaxDiagramBytes bounds a diagram's DECODED size (02:GEN-005). Same
// number as contracts/openapi/public.yaml's GenerateDiagram.data
// x-max-decoded-bytes and llm-internal.yaml's GenerateDiagram.
const generateMaxDiagramBytes = 4_000_000 // one-number: generateMaxDiagramBytes

// generateMaxReferences bounds how many existing Skills the model reads as
// worked examples in one generation (02:GEN-006).
const generateMaxReferences = 3 // one-number: generateMaxReferences

// generateMaxReferenceChars caps one reference's SKILL.md, in runes, the same
// way improvement/suggest.go's firstChars caps a target file: cut, not
// dropped, so the model still sees the start of a document that ran long.
const generateMaxReferenceChars = 20000 // one-number: generateMaxReferenceChars

// referenceTruncationMarker is appended to a reference's SKILL.md when it is
// cut for generateMaxReferenceChars. Named as a const rather than an inline
// literal because it has to be subtracted from the cap BEFORE cutting: cutting
// to the full cap and then appending twelve more runes put a truncated
// reference over the cap apps/llm's own `skill_md` schema enforces
// (max_length=20000), and apps/llm answered 422 to a request Go itself built
// oversized — reaching the user as the generic 502 「模型服務這一次沒有給出可用
// 的結果」, indistinguishable from an actual gateway failure.
const referenceTruncationMarker = "…[truncated]"

// classifyTaskDescription is 02:GEN-001/005's whole bound-checking rule for
// the task description, pulled out of GenerateSkill so it can be tested
// without a Pool: task is already trimmed, hasDiagram says whether a diagram
// travels alongside it.
func classifyTaskDescription(task string, hasDiagram bool) error {
	n := len([]rune(task))
	switch {
	case n == 0 && !hasDiagram:
		return ErrGenerateNoInput
	case n > maxTaskDescriptionRunes:
		return ErrGenerateTooLong
	case n > 0 && n < minTaskDescriptionRunes && !hasDiagram:
		return ErrGenerateBlank
	}
	return nil
}

// GenerateDiagram is a decoded diagram image, admission's own shape —
// llmclient.GenerateDiagram carries the base64 text instead, because only the
// wire format needs to be text.
type GenerateDiagram struct {
	MediaType string
	Data      []byte
}

// GenerateInput is everything GenerateSkill can be asked to generate from
// (02:GEN-001/005/006). At least one of TaskDescription/Diagram must survive
// trimming, checked in GenerateSkill itself — a product rule, so it lives in
// Go and not in the request decoder (iron rule 6).
type GenerateInput struct {
	TaskDescription   string
	Diagram           *GenerateDiagram
	ReferenceSkillIDs []pgtype.UUID
}

// ReferenceReader is the read slice ingest needs from registry to resolve a
// reference skill id (02:GEN-006, ADR-034 style narrow interface). *registry.Service
// already has exactly these three methods; the composition root wires it in
// (apiserver/app.go) rather than this package importing registry for a
// concrete type it would then have to construct itself (platform-ddd-practices
// 跨 Context 協作).
type ReferenceReader interface {
	WorkspaceSkill(ctx context.Context, workspaceID, skillID pgtype.UUID) (registry.Skill, bool, error)
	CatalogSkill(ctx context.Context, skillID pgtype.UUID) (registry.Skill, bool, error)
	LatestVersion(ctx context.Context, workspaceID, skillID pgtype.UUID) (registry.Version, bool, error)
}

// ErrGeneratedPackageInvalid: the answer parsed but cannot be made into an
// archive at all. Distinct from a blocked report — there is no Report to show —
// and not retried, because the model returned a shape the schema said it could
// not return, and the same prompt asks for the same shape.
var ErrGeneratedPackageInvalid = errors.New("ingest: generated skill cannot be packaged")

// ErrGeneratedNameCollision: this workspace already has a skill of that name
// and at least one of the two is generated. See importZip for why merging them
// is worse than refusing.
var ErrGeneratedNameCollision = errors.New(
	"ingest: this workspace already has a skill with that name, and at least one of the two is generated")

// The failure vocabulary auditGenerateFailure writes and GenerateFailures reads
// back. Constants rather than literals because the same words also appear in
// contracts/openapi/public.yaml's GenerationFailure.failure enum and in the web
// sentence table, and a word changed in one place and not the others is a row
// the screen calls unreadable. generate_integration_test asserts every one of
// these is in the generated enum.
const (
	FailureQuota         = "quota"
	FailureUnavailable   = "unavailable"
	FailureGateway       = "gateway"
	FailureUnpackageable = "unpackageable"
	FailureRejected      = "rejected"
	FailureBlocked       = "blocked"
)

// FailureVocabulary is every value this package writes, for the test above.
var FailureVocabulary = []string{
	FailureQuota, FailureUnavailable, FailureGateway, FailureUnpackageable, FailureRejected, FailureBlocked,
}

// GenerateResult is one generation. Result.Report is the validation outcome and
// is shown to the user verbatim on failure (02:GEN-003, same treatment SKILL-002
// already gives a failed import); Result.Version is meaningless when
// Report.Blocked is true, exactly as it is for UploadZip.
type GenerateResult struct {
	Result
	// Attempts is 1 or 2. Worth reporting rather than inferring: "it took two
	// goes" is the difference between the measured 80% and the measured 90%.
	Attempts int
	// Model and PromptVersion are what answered, echoed back so a caller can
	// show provenance without re-reading the source row.
	Model         string
	PromptVersion string
	// CostUSD is what the gateway charged for this generation, summed over its
	// attempts, and nil when the gateway priced nothing.
	//
	// Nil is not zero. 02:GEN-001 wants a cost estimate before generation and
	// 05 R-10 concluded that what it is waiting for is a batch of generations
	// that each recorded their own cost — round B recorded an average, and
	// 02:PDM-005 §2.2 forbids printing an average as an estimate because one
	// long description can cost several times another. A zero written where the
	// gateway reported nothing would enter that distribution as a real
	// observation and drag it, which is worse than a gap.
	CostUSD *float64
	// PromptTokens and CompletionTokens are the same total in the unit that
	// survives a deployment whose gateway prices nothing at all.
	PromptTokens     int64
	CompletionTokens int64
}

// addUsage folds one gateway call's reported usage into the generation's total.
//
// A generation is up to two calls (generateMaxAttempts), and what it cost is
// what all of them cost — the retry is not free and must not be invisible.
//
// Cost is taken only when the gateway itself priced the call: `cost_source` is
// how apps/llm says whether the number came from LiteLLM or from something it
// worked out, and eval's suggest leg already drops a cost from any other source
// before it stores one. Same rule here, so the two legs' numbers mean the same
// thing when somebody puts them side by side.
func (r *GenerateResult) addUsage(u *llmclient.GatewayUsage) {
	if u == nil {
		return
	}
	r.PromptTokens += u.PromptTokens
	r.CompletionTokens += u.CompletionTokens
	if u.CostUSD == nil || u.CostSource != "gateway" {
		return
	}
	total := *u.CostUSD
	if r.CostUSD != nil {
		total += *r.CostUSD
	}
	r.CostUSD = &total
}

// GenerateSkill writes one Skill from a task description, a diagram, existing
// reference Skills, or a combination, and imports it (GEN-001, GEN-003,
// 02:GEN-005, 02:GEN-006).
//
// Nothing here is executed and nothing inside the package is either, at any
// point: generation and validation are static analysis over bytes the same way
// import is, and any Script the model wrote only ever runs in a Sandbox
// (iron rule 1, 02:GEN-003).
func (s *Service) GenerateSkill(ctx context.Context, ws identity.Workspace, in GenerateInput) (GenerateResult, error) {
	if s.LLM == nil {
		return GenerateResult{}, errors.New("ingest: generation needs an LLM service")
	}
	// GEN-007 keys the search exclusion on redistribution, and redistributionFor
	// answers "" for a catalog workspace before it ever looks at the source type
	// — so a package generated into the catalogue would be `unknown`, would not
	// match that exclusion, and would be searchable. Refusing here is one line
	// and keeps `redistribution = generated` and `source_type = generated`
	// meaning the same set of rows. It costs nothing: ADR-046 決策 1 puts
	// generated content in a personal workspace and nowhere else.
	if ws.IsCatalog {
		return GenerateResult{}, ErrGenerateNotForCatalogue
	}
	task := strings.TrimSpace(in.TaskDescription)
	// Both bounds in Go, because both are product rules and only one of them was
	// here. apps/llm enforces `min_length=8, max_length=4000` on its own side, so
	// a three-character description used to travel all the way there, come back a
	// Pydantic 422, and reach the user as **502 「generation failed」** — the
	// platform reporting itself broken when the fix was "write a bit more".
	// Counted in runes: 「整理發票」 is four characters and eight bytes.
	//
	// GEN-005 loosened the floor without removing it: a description is either
	// absent (fine, as long as a diagram carries the task) or held to the same
	// eight-rune minimum it always was. The floor exists to stop a bare box, not
	// to force a caption up to eight runes when a diagram is doing the actual
	// work — so beside a diagram any non-blank caption is passed through as
	// context and the diagram carries the task; alone, eight runes is still the
	// minimum (see classifyTaskDescription, TestAShortCaptionWithADiagramIsNotBlank).
	if err := classifyTaskDescription(task, in.Diagram != nil); err != nil {
		return GenerateResult{}, err
	}

	if in.Diagram != nil {
		if !validDiagramMediaType(in.Diagram.MediaType) ||
			len(in.Diagram.Data) == 0 || len(in.Diagram.Data) > generateMaxDiagramBytes {
			return GenerateResult{}, ErrDiagramInvalid
		}
	}

	if len(in.ReferenceSkillIDs) > generateMaxReferences {
		return GenerateResult{}, ErrTooManyReferences
	}
	// Resolved and read BEFORE the allowance check and the gateway (02:GEN-006):
	// an id the caller cannot use must not cost anything, the same rule
	// ErrGenerateBlank already follows for the task description.
	var references []llmclient.GenerateReference
	var refProvenance []referenceProvenance
	if len(in.ReferenceSkillIDs) > 0 {
		if s.References == nil {
			return GenerateResult{}, ErrReferenceUnavailable
		}
		for _, id := range in.ReferenceSkillIDs {
			ref, prov, err := s.resolveReference(ctx, ws, id)
			if err != nil {
				return GenerateResult{}, err
			}
			references = append(references, ref)
			refProvenance = append(refProvenance, prov)
		}
	}
	generationInputsJSON, err := marshalGenerationInputs(in.Diagram, refProvenance)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("ingest: generation_inputs: %w", err)
	}

	// One generation per workspace at a time. It bounds CONCURRENCY and not rate:
	// GENERATE_QUOTA is `off` in the shipped config, the invite list is empty
	// there too, and the allowance — when it is on — reads a live count that
	// concurrent callers all see the same value of. Without this one session can
	// hold an unbounded number of paid calls open at once, and they all draw on
	// the SHARED gateway key: exhausting it stops search enrichment and every
	// LLM judge too, because those use the same key.
	//
	// It is no longer the only brake, and this comment used to say it was: 403b385
	// wrapped POST /skills/generate in limited() (router.go), so the sequential
	// loop this could never stop is now stopped by the per-IP token bucket that
	// the sentence here claimed did not exist anywhere in the service. Neither
	// replaces the other — the bucket counts requests over time, this counts them
	// at one instant, and a limiter can also be turned off with RATE_LIMIT=off.
	//
	// ponytail: in-process, so it bounds one API replica and not a fleet. That is
	// the right size for today (one cmd/api) and the wrong size the day there are
	// two; the durable version is a rate limit at the edge, tracked in 04. It is
	// a brake, not a quota — the quota is policy's.
	if !s.holdGenerateSlot(ws.ID) {
		return GenerateResult{}, ErrGenerateInFlight
	}
	defer s.releaseGenerateSlot(ws.ID)

	// Before the first gateway call and before anything is written: 02:GEN-001
	// 「額度不足時在呼叫模型之前拒絕，不得先花錢再說」.
	if reason, err := s.requireGenerateAllowance(ctx, ws.ID); err != nil {
		// Two different things refuse here and the record has to say which. An
		// exhausted allowance is `quota`; an allowance that could not be COUNTED
		// is `unavailable` — the 422 path stopped conflating them (d555564), and
		// the failure list a user reads back must not be the place that tells a
		// healthy account it ran out.
		failure := FailureQuota
		if errors.Is(err, policy.ErrAllowanceUnavailable) {
			failure = FailureUnavailable
		}
		// GenerateResult{}: refused before the gateway, so no attempt and no cost.
		s.auditGenerateFailure(ctx, ws, task, in, GenerateResult{}, map[string]any{
			"failure": failure,
			"reason":  reason,
		})
		return GenerateResult{}, err
	}

	var out GenerateResult
	// One line per generation that reached the gateway, whatever became of it.
	// The audit row below covers the failures; this covers the successes, which
	// are the ones 04 丙-53's distribution is mostly made of and the ones that
	// currently have nowhere durable to land (see auditGenerateFailure).
	//
	// Deferred rather than written at each return: there are five of them, and a
	// counter that only some paths emit is the defect 04 丙-38 was about.
	defer func() {
		if out.Attempts == 0 {
			return
		}
		attrs := []any{
			"attempts", out.Attempts, "model", out.Model,
			"prompt_tokens", out.PromptTokens, "completion_tokens", out.CompletionTokens,
		}
		// Omitted rather than logged as zero, for the reason CostUSD is a pointer.
		if out.CostUSD != nil {
			attrs = append(attrs, "cost_usd", *out.CostUSD)
		}
		slog.Info("generate: model usage", attrs...)
	}()
	for attempt := 1; attempt <= generateMaxAttempts; attempt++ {
		out.Attempts = attempt

		// Same prompt, same model, no correction hint added from the first
		// failure (ADR-047 決策 1). Feeding the findings back would be a second
		// prompt, and then the provenance row no longer reproduces the package.
		gen, err := s.generateOnce(ctx, task, in.Diagram, references)
		if err != nil {
			// Logged as well as audited. The audit row records that a generation
			// failed at the gateway; nothing recorded WHY, so a deployment failing
			// 100% of the time (wrong key, model name typo, budget exhausted,
			// apps/llm down) produced four identical 502s and no way to tell them
			// apart (NFR-003 「內部診斷碼」). Same shape enrich.go uses.
			slog.Warn("generate: gateway call failed", "attempt", attempt, "error", err)
			// Truncation arrives here too and is not retried: the ceiling covers
			// reasoning plus output, so a second call at the same ceiling buys
			// the same answer (ADR-047 決策 2). Neither is a gateway refusal,
			// for the reason SuggestImprovements already gives — a gateway that
			// refused this input refuses it again.
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure":   FailureGateway,
				"truncated": errors.Is(err, llmclient.ErrGenerateTruncated),
			})
			return out, err
		}
		out.Model, out.PromptVersion = gen.Model, gen.PromptVersion
		// Before anything downstream can fail: a call that came back was paid for
		// whether or not what it returned can be packaged.
		out.addUsage(gen.Usage)

		data, err := buildGeneratedPackage(gen.Skill)
		if err != nil {
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{"failure": FailureUnpackageable})
			return out, err
		}

		desc, model, promptVersion := task, gen.Model, gen.PromptVersion
		res, err := s.importZip(ctx, ws, data, sourceMeta{
			Type:                   sourceGenerated,
			TaskDescription:        &desc,
			GeneratorModel:         &model,
			GeneratorPromptVersion: &promptVersion,
			// Read after addUsage above, so a retried generation carries what
			// both attempts cost rather than what the last one did.
			CostUSD:          out.CostUSD,
			PromptTokens:     out.PromptTokens,
			CompletionTokens: out.CompletionTokens,
			// nil unless a diagram or references were behind this generation
			// (ADR-066); the image bytes themselves are never stored.
			GenerationInputs: generationInputsJSON,
		})
		if err != nil {
			// A refused generation is still a paid one, and 02:GEN-003 asks for a
			// record of it. The name collision arrives here rather than as a
			// blocked report, so without this the one failure a user can actually
			// act on was the one that left nothing behind.
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure":   FailureRejected,
				"collision": errors.Is(err, ErrGeneratedNameCollision),
			})
			return out, err
		}
		out.Result = res
		if !res.Report.Blocked {
			return out, nil
		}

		// A blocked report leaves nothing behind: prepare returns before the
		// object store write and importZip returns before the transaction opens,
		// so there is no half-made version to clean up (02:GEN-003 「不得留下一個
		// 半成品版本」).
		if !shouldRetry(attempt, res.Report) {
			s.auditGenerateFailure(ctx, ws, task, in, out, map[string]any{
				"failure": FailureBlocked,
				// Codes and nothing else. A finding's Message never carries the
				// matched value (skillpkg.go:909) and this must not become the
				// place that reintroduces it (NFR-002, iron rule 11).
				"codes": blockingCodes(res.Report),
			})
			return out, nil
		}
	}
	return out, nil // unreachable: the loop returns on every path
}

// shouldRetry is the whole retry policy: one more attempt at the same prompt,
// unless a credential-shaped line was found.
//
// The exception is not a special case bolted on — it is the one blocking code of
// the twelve that reads file content rather than package structure. The other
// eleven catch a slip in how the answer was laid out, and a second attempt
// usually lays it out correctly; a model that wrote `aws_secret_access_key=...`
// wrote it because the task made it seem useful, and the same prompt makes it
// seem useful again (ADR-048). When both kinds appear at once, not retrying
// wins (02:GEN-003).
func shouldRetry(attempt int, r skillpkg.Report) bool {
	return attempt < generateMaxAttempts &&
		!slices.Contains(blockingCodes(r), skillpkg.CodePossibleSecret)
}

// generateTimeout bounds ONE attempt at the gateway.
//
// llmclient deliberately imposes no timeout of its own — the deadline is the
// caller's ctx and nothing else — and this caller was the one that forgot. A
// generation request had no deadline anywhere in Go: not here, not in the client,
// and cmd/api sets only ReadHeaderTimeout. A stalled apps/llm (a rolling
// container, a blackholed address that still resolves) therefore pinned the
// goroutine and the connection until the browser gave up, and a caller that never
// gives up pinned them for good.
//
// Just above apps/llm's own 120s socket ceiling, the same rule enrich.go states:
// the upstream's limit has to surface as its error rather than as our deadline,
// or the work is billed and thrown away. Per ATTEMPT, so the retry does not
// inherit what the first attempt spent.
// budget-over: generate.LLM_TIMEOUT_SECONDS
const generateTimeout = 130 * time.Second

func (s *Service) generateOnce(
	ctx context.Context, task string, diagram *GenerateDiagram, references []llmclient.GenerateReference,
) (*llmclient.GenerateSkillResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	req := llmclient.GenerateSkillRequest{TaskDescription: task, References: references}
	if diagram != nil {
		// Standard base64 (RFC 4648 §4), the encoding public.yaml's GenerateDiagram
		// and llm-internal.yaml's both name explicitly.
		req.Diagram = &llmclient.GenerateDiagram{
			MediaType: diagram.MediaType,
			Data:      base64.StdEncoding.EncodeToString(diagram.Data),
		}
	}
	return s.LLM.GenerateSkill(callCtx, req)
}

// validDiagramMediaType is the three formats 02:GEN-005 and GenerateDiagram's
// contract enum accept. SVG is deliberately not one of them: it is text that
// can carry scripts, and the diagram path must not become a second import
// path (iron rule 1).
func validDiagramMediaType(mediaType string) bool {
	switch mediaType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

// referenceProvenance is what a resolved reference skill leaves in
// generation_inputs (ADR-066) — identifiers only, never its SKILL.md content.
type referenceProvenance struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
	Name      string
}

// resolveReference reads one 02:GEN-006 reference skill id into the SKILL.md
// text the model sees and the identifiers the provenance row keeps.
//
// Scope order is Fork's (registry.go): the caller's own workspace first, then
// the public catalogue — so a caller's private skill is never shadowed by a
// catalogue skill sharing an id. Refused, as ErrReferenceUnavailable, for
// anything not found in either scope, taken down, under a licensing hold, or
// `redistribution = blocked` — the same four words 02:GEN-006 names — and for
// a stored package this side cannot read back, because a reference silently
// skipped is a reference the caller was told was used and was not.
func (s *Service) resolveReference(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID,
) (llmclient.GenerateReference, referenceProvenance, error) {
	skill, found, err := s.References.WorkspaceSkill(ctx, ws.ID, id)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, err
	}
	if !found {
		skill, found, err = s.References.CatalogSkill(ctx, id)
		if err != nil {
			return llmclient.GenerateReference{}, referenceProvenance{}, err
		}
	}
	if !found || skill.TakedownAt.Valid || skill.AccessRestriction != nil || skill.Redistribution == "blocked" {
		return llmclient.GenerateReference{}, referenceProvenance{}, ErrReferenceUnavailable
	}

	version, found, err := s.References.LatestVersion(ctx, skill.WorkspaceID, skill.ID)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, err
	}
	if !found {
		return llmclient.GenerateReference{}, referenceProvenance{}, ErrReferenceUnavailable
	}

	data, err := s.Store.Get(ctx, version.PackageObjectKey)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}
	md, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		return llmclient.GenerateReference{}, referenceProvenance{}, fmt.Errorf("%w: %v", ErrReferenceUnavailable, err)
	}

	// The marker's own length is subtracted from the cap before cutting, so
	// content+marker together never exceed generateMaxReferenceChars — see
	// referenceTruncationMarker.
	content, truncated := cutRunes(strings.ToValidUTF8(string(md), ""),
		generateMaxReferenceChars-utf8.RuneCountInString(referenceTruncationMarker))
	if truncated {
		content += referenceTruncationMarker
	}
	return llmclient.GenerateReference{Name: skill.Name, SkillMD: content},
		referenceProvenance{SkillID: skill.ID, VersionID: version.ID, Name: skill.Name}, nil
}

// cutRunes is judge.go's `cut` restated here: this package does not import
// trial/improvement for one four-line helper, and a shared home for it is not
// this brief's problem to solve.
func cutRunes(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

// generationInputsDiagram is generation_inputs' `diagram` object (0055,
// ADR-066): a digest and a size, never the bytes.
type generationInputsDiagram struct {
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
}

// generationInputsReference is one entry of generation_inputs' `references`
// array: identifiers only, the same restraint referenceProvenance keeps.
type generationInputsReference struct {
	SkillID   string `json:"skill_id"`
	VersionID string `json:"version_id"`
	Name      string `json:"name"`
}

// generationInputsRecord is the whole shape of `skill_sources.generation_inputs`
// (0055 migration comment, ADR-066).
type generationInputsRecord struct {
	Diagram    *generationInputsDiagram    `json:"diagram,omitempty"`
	References []generationInputsReference `json:"references,omitempty"`
}

// marshalGenerationInputs builds generation_inputs, or returns nil when
// neither a diagram nor references were behind this generation — the column
// is nullable exactly so a plain task-description generation writes nothing
// new.
func marshalGenerationInputs(diagram *GenerateDiagram, refs []referenceProvenance) ([]byte, error) {
	if diagram == nil && len(refs) == 0 {
		return nil, nil
	}
	rec := generationInputsRecord{}
	if diagram != nil {
		sum := sha256.Sum256(diagram.Data)
		rec.Diagram = &generationInputsDiagram{
			MediaType: diagram.MediaType,
			SHA256:    hex.EncodeToString(sum[:]),
			Bytes:     len(diagram.Data),
		}
	}
	for _, r := range refs {
		rec.References = append(rec.References, generationInputsReference{
			SkillID: pgconv.UUIDString(r.SkillID), VersionID: pgconv.UUIDString(r.VersionID), Name: r.Name,
		})
	}
	return json.Marshal(rec)
}

// blockingCodes lists the distinct error-level codes, in report order.
func blockingCodes(r skillpkg.Report) []string {
	var codes []string
	for _, f := range r.Findings {
		if f.Severity == skillpkg.SeverityError && !slices.Contains(codes, f.Code) {
			codes = append(codes, f.Code)
		}
	}
	return codes
}

// generatedFrontmatter is the SKILL.md frontmatter Go writes for the model.
//
// Field order here is the file's field order, which is why it is a struct and
// not a map. `license` is absent and cannot be added by anything the model
// returns: llmclient.GeneratedSkill has no such field either (ADR-046 決策 5).
// `metadata` is absent for a duller reason — see llmclient.GeneratedSkill.
type generatedFrontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Compatibility string `yaml:"compatibility,omitempty"`
	AllowedTools  string `yaml:"allowed-tools,omitempty"`
}

// buildGeneratedPackage turns the model's answer into a package archive.
//
// Go serialises the frontmatter; the model never writes a YAML key. That is the
// whole reason the endpoint returns fields instead of a SKILL.md string: across
// 59 measured generations every blocking failure was a damaged *key* — a leading
// space, an Arabic letter glued to `description`, `说明` in its place — and a key
// the model does not write cannot be damaged. Two of the twelve blocking codes
// stop being reachable rather than merely unlikely, and the damaged-key route to
// a third (description-missing) closes with them; that one stays reachable
// through an empty value, and skillpkg reports it.
//
// The body and the extra files are copied byte for byte. ADR-047 決策 1 says the
// platform does not edit what the model returned, and that rule is only
// meaningful if the recorded task description, model and prompt version really do
// reproduce what is sitting in the workspace.
func buildGeneratedPackage(g llmclient.GeneratedSkill) ([]byte, error) {
	fm, err := yaml.Marshal(generatedFrontmatter{
		Name:          g.Name,
		Description:   g.Description,
		Compatibility: g.Compatibility,
		AllowedTools:  g.AllowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: frontmatter: %v", ErrGeneratedPackageInvalid, err)
	}

	var md bytes.Buffer
	md.WriteString("---\n")
	md.Write(fm)
	md.WriteString("---\n\n")
	md.WriteString(g.Body)
	if !strings.HasSuffix(g.Body, "\n") {
		md.WriteString("\n")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("%w: entry %q: %v", ErrGeneratedPackageInvalid, name, err)
		}
		_, err = io.WriteString(w, content)
		return err
	}
	if err := write("SKILL.md", md.String()); err != nil {
		return nil, err
	}
	for _, f := range g.Files {
		// Compared on the NAME THE READER WILL SEE, not on the string the model
		// wrote. archive/zip's fs view runs path.Clean over entry names, so
		// `SKILL.md/`, `.//SKILL.md` and `././SKILL.md` all resolve to SKILL.md
		// while looking nothing like it here — and none of them is an
		// entry-path-escape, so nothing else catches them either. The result of
		// missing one is not a collision the user is told about: it is
		// `skill-md-missing` on a package that visibly contains SKILL.md, plus a
		// second paid attempt at the same thing.
		clean := path.Clean(strings.ReplaceAll(f.Path, `\`, "/"))
		if strings.EqualFold(clean, "SKILL.md") {
			return nil, fmt.Errorf("%w: a second SKILL.md at %q", ErrGeneratedPackageInvalid, f.Path)
		}
		// "." and "" resolve to the archive root. Such an entry is not an escape,
		// so ArchiveEntryFinding passes it; it lands inside content_hash and
		// inside the stored archive, and then every disclosure surface skips it —
		// scanTree never opens it, so `possible-secret` and the script disclosure
		// never see it, and delivery's exporter neither ships it nor lists it as
		// dropped. Model-authored bytes that no warning covers is exactly what
		// 02:GEN-003 forbids.
		if clean == "." || clean == "" || clean == "/" {
			return nil, fmt.Errorf("%w: entry %q names no file", ErrGeneratedPackageInvalid, f.Path)
		}
		// Escaping and absolute paths are deliberately NOT filtered here. They
		// are read off the raw archive entry names by skillpkg.ArchiveEntryFinding
		// and come back as a blocking finding, which is both the same treatment an
		// uploaded package gets and the treatment 02:GEN-003 asks for — no
		// warning removed and no risk downgraded because the platform wrote it.
		if err := write(f.Path, f.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneratedPackageInvalid, err)
	}
	return buf.Bytes(), nil
}

// auditGenerateFailure leaves the workspace something to look at after a
// generation that produced nothing (02:GEN-003 「在工作區留下可查的失敗紀錄」).
//
// Its own transaction, because the import transaction either never opened or
// already rolled back. Best-effort: a failed audit write must not turn a failed
// generation into a different error, and slog is the fallback record.
//
// The task description is NOT recorded here. It is user free text and it belongs
// to the skill_sources row, under NFR-002 deletion; audit rows are kept for 400
// days under a different rule, and one copy under each is a retention promise
// nobody made (ADR-029 decision 3 draws the same line for analytics).
func (s *Service) auditGenerateFailure(
	ctx context.Context, ws identity.Workspace, task string, in GenerateInput, out GenerateResult, meta map[string]any,
) {
	meta["attempts"] = out.Attempts
	meta["task_description_chars"] = len([]rune(task))
	// Never content (iron rule 11): a bool and a count, the same restraint
	// task_description_chars already keeps for the text.
	meta["diagram"] = in.Diagram != nil
	meta["references"] = len(in.ReferenceSkillIDs)
	usageMeta(meta, out.CostUSD, out.PromptTokens, out.CompletionTokens)
	// WithoutCancel: the commonest failure is the gateway call running out of the
	// caller's deadline, and that same dead ctx would take the record with it.
	//
	// Written straight to the pool, not through a transaction of this package's
	// own: audit.Log takes a DBTX and *pgxpool.Pool is one, so a single INSERT
	// gets its implicit transaction from the server. Opening one here bought two
	// error branches no test can reach and nothing else.
	ctx = context.WithoutCancel(ctx)
	if err := audit.Log(ctx, s.Pool, audit.Event{
		Actor:     ws.OwnerUserID,
		Workspace: ws.ID,
		Action:    audit.ActionSkillGenerateFailed,
		// No resource_id: the point of the row is that no skill was created.
		// Same shape ActionOperatorRoster already uses for a row nothing points at.
		ResourceType: audit.ResourceSkill,
		Metadata:     meta,
	}); err != nil {
		slog.Error("generate: failure record not written", "error", err)
	}
}

// usageMeta adds what a model call cost to an audit row's metadata.
//
// Admissible under iron rule 11 by the same reading its own package doc gives:
// audit metadata carries identifiers and outcome, never package content, prompts
// or secrets. A token count and a dollar amount are neither — they say nothing
// about what the user asked for or what the model wrote, which is why
// task_description_chars is already here and the description itself is not.
//
// It is the durable half of 04 丙-53, and it is deliberately the ONLY place the
// rule lives. Both halves of the sample go through it: a failed generation via
// auditGenerateFailure, a successful one via the ordinary skill.import row
// importZip writes. Two copies of "absent means absent" would eventually
// disagree, and the disagreement would look like a costing anomaly rather than
// like the bug it is.
//
// An unreported cost leaves the key out. A row saying `cost_usd: 0` is a claim
// that the call was free; a row with no such key is the absence it actually is,
// and the reader of a distribution has to be able to tell those apart.
func usageMeta(meta map[string]any, cost *float64, prompt, completion int64) {
	if cost != nil {
		meta["cost_usd"] = *cost
	}
	if prompt > 0 || completion > 0 {
		meta["prompt_tokens"] = prompt
		meta["completion_tokens"] = completion
	}
}

// requireGenerateAllowance is the generation half of ADR-028's enforcement
// points (GEN-004). The rule is policy's, the point is here: `policy` decides
// whether there is allowance left, this package is what asks and what refuses.
//
// Nil GenerateQuota, or a zero one, means this deployment enforces no generation
// allowance — it then refuses nothing and claims none exists, the same pairing
// QuotaLimits.Enforced already defines for runs (04 乙-2: a number on a screen is
// a claim that it is applied).
//
// The read runs in its own short transaction because identity.ReadWorkspaceCreatedAt
// needs one. It is not the transaction that writes the result and cannot be: the
// model call happens in between, and holding a connection open across it would
// cost far more than the overshoot it would prevent.
func (s *Service) requireGenerateAllowance(ctx context.Context, workspaceID pgtype.UUID) (string, error) {
	if !s.GenerateQuota.Enforced() {
		return "", nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		slog.Error("generate: the allowance could not be counted", "error", err)
		return "generate_quota_unavailable", fmt.Errorf("%w: %w", policy.ErrAllowanceUnavailable, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	return policy.EnforceGenerateQuota(ctx, generateUsageReader(tx), s.GenerateQuota, workspaceID)
}

// generateUsageReader counts generations the way policy counts runs. One
// definition, because the enforcement path and the display path disagreeing
// about the number is the failure 04 乙-2 describes: a screen that promises an
// allowance the gate does not apply.
func generateUsageReader(tx pgx.Tx) policy.UsageReader {
	q := gen.New(tx)
	return policy.UsageReader{
		WorkspaceCreatedAt: func(ctx context.Context, id pgtype.UUID) (time.Time, error) {
			return identity.ReadWorkspaceCreatedAt(ctx, tx, id)
		},
		CountRuns: func(ctx context.Context, id pgtype.UUID, since time.Time) (policy.RunUsage, error) {
			row, err := q.CountGeneratedSkills(ctx, gen.CountGeneratedSkillsParams{
				WorkspaceID: id, Since: pgtype.Timestamptz{Time: since, Valid: true},
			})
			if err != nil {
				return policy.RunUsage{}, err
			}
			u := policy.RunUsage{Used: row.Used}
			if row.Oldest.Valid {
				oldest := row.Oldest.Time
				u.Oldest = &oldest
			}
			return u, nil
		},
	}
}

// holdGenerateSlot takes this workspace's single generation slot, reporting
// false when somebody already has it. See GenerateSkill for why it exists and
// what it does not cover.
func (s *Service) holdGenerateSlot(workspaceID pgtype.UUID) bool {
	_, busy := s.generating.LoadOrStore(workspaceID, struct{}{})
	return !busy
}

func (s *Service) releaseGenerateSlot(workspaceID pgtype.UUID) {
	s.generating.Delete(workspaceID)
}
