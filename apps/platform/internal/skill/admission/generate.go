package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"gopkg.in/yaml.v3"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// Generation from a task description (GEN-003, ADR-046).
//
// It lives in ingest and not next to the other LLM callers because the thing it
// produces is an import: the bytes go through prepare → Validate → enrich → tx
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

// ErrGeneratedPackageInvalid: the answer parsed but cannot be made into an
// archive at all. Distinct from a blocked report — there is no Report to show —
// and not retried, because the model returned a shape the schema said it could
// not return, and the same prompt asks for the same shape.
var ErrGeneratedPackageInvalid = errors.New("ingest: generated skill cannot be packaged")

// ErrGeneratedNameCollision: this workspace already has a skill of that name,
// and the two are not the same kind of content. See importZip for why merging
// them is worse than refusing.
var ErrGeneratedNameCollision = errors.New(
	"ingest: this workspace already has a skill with that name, and one of the two was generated")

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
}

// GenerateSkill writes one Skill from a task description and imports it
// (GEN-001, GEN-003).
//
// Nothing here is executed and nothing inside the package is either, at any
// point: generation and validation are static analysis over bytes the same way
// import is, and any Script the model wrote only ever runs in a Sandbox
// (iron rule 1, 02:GEN-003).
func (s *Service) GenerateSkill(ctx context.Context, ws identity.Workspace, taskDescription string) (GenerateResult, error) {
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
	task := strings.TrimSpace(taskDescription)
	// Both bounds in Go, because both are product rules and only one of them was
	// here. apps/llm enforces `min_length=8, max_length=4000` on its own side, so
	// a three-character description used to travel all the way there, come back a
	// Pydantic 422, and reach the user as **502 「generation failed」** — the
	// platform reporting itself broken when the fix was "write a bit more".
	// Counted in runes: 「整理發票」 is four characters and eight bytes.
	if n := len([]rune(task)); n == 0 || n < minTaskDescriptionRunes {
		return GenerateResult{}, ErrGenerateBlank
	} else if n > maxTaskDescriptionRunes {
		return GenerateResult{}, ErrGenerateTooLong
	}

	// One generation per workspace at a time. It is the only brake that exists on
	// how MANY generations a session can start: GENERATE_QUOTA is off, the invite
	// list is empty in the shipped config, no rate limiter exists anywhere in this
	// service, and the allowance — when it is on — reads a live count that
	// concurrent callers all see the same value of. Without this one session can
	// hold an unbounded number of paid calls open at once, and they all draw on
	// the SHARED gateway key: exhausting it stops search enrichment and every
	// LLM judge too, because those use the same key.
	//
	// ponytail: in-process, so it bounds one API replica and not a fleet. That is
	// the right size for today (one cmd/api) and the wrong size the day there are
	// two; the durable version is a rate limit at the edge (NFR-001 clause 5),
	// tracked in 04. It is a brake, not a quota — the quota is policy's.
	if !s.holdGenerateSlot(ws.ID) {
		return GenerateResult{}, ErrGenerateInFlight
	}
	defer s.releaseGenerateSlot(ws.ID)

	// Before the first gateway call and before anything is written: 02:GEN-001
	// 「額度不足時在呼叫模型之前拒絕，不得先花錢再說」.
	if reason, err := s.requireGenerateAllowance(ctx, ws.ID); err != nil {
		s.auditGenerateFailure(ctx, ws, task, 0, map[string]any{
			"failure": "quota",
			"reason":  reason,
		})
		return GenerateResult{}, err
	}

	var out GenerateResult
	for attempt := 1; attempt <= generateMaxAttempts; attempt++ {
		out.Attempts = attempt

		// Same prompt, same model, no correction hint added from the first
		// failure (ADR-047 決策 1). Feeding the findings back would be a second
		// prompt, and then the provenance row no longer reproduces the package.
		gen, err := s.generateOnce(ctx, task)
		if err != nil {
			// Logged as well as audited. The audit row records that a generation
			// failed at the gateway; nothing recorded WHY, so a deployment failing
			// 100% of the time (wrong key, model name typo, budget exhausted,
			// apps/llm down) produced four identical 502s and no way to tell them
			// apart (NFR-003 「內部診斷碼」). Same shape enrich.go已經 uses.
			slog.Warn("generate: gateway call failed", "attempt", attempt, "error", err)
			// Truncation arrives here too and is not retried: the ceiling covers
			// reasoning plus output, so a second call at the same ceiling buys
			// the same answer (ADR-047 決策 2). Neither is a gateway refusal,
			// for the reason SuggestImprovements already gives — a gateway that
			// refused this input refuses it again.
			s.auditGenerateFailure(ctx, ws, task, attempt, map[string]any{
				"failure":   "gateway",
				"truncated": errors.Is(err, llmclient.ErrGenerateTruncated),
			})
			return out, err
		}
		out.Model, out.PromptVersion = gen.Model, gen.PromptVersion

		data, err := buildGeneratedPackage(gen.Skill)
		if err != nil {
			s.auditGenerateFailure(ctx, ws, task, attempt, map[string]any{"failure": "unpackageable"})
			return out, err
		}

		desc, model, promptVersion := task, gen.Model, gen.PromptVersion
		res, err := s.importZip(ctx, ws, data, sourceMeta{
			Type:                   sourceGenerated,
			TaskDescription:        &desc,
			GeneratorModel:         &model,
			GeneratorPromptVersion: &promptVersion,
		})
		if err != nil {
			// A refused generation is still a paid one, and 02:GEN-003 asks for a
			// record of it. The name collision arrives here rather than as a
			// blocked report, so without this the one failure a user can actually
			// act on was the one that left nothing behind.
			s.auditGenerateFailure(ctx, ws, task, attempt, map[string]any{
				"failure":   "rejected",
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
			s.auditGenerateFailure(ctx, ws, task, attempt, map[string]any{
				"failure": "blocked",
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
const generateTimeout = 130 * time.Second

func (s *Service) generateOnce(ctx context.Context, task string) (*llmclient.GenerateSkillResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, generateTimeout)
	defer cancel()
	return s.LLM.GenerateSkill(callCtx, task)
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
// the model does not write cannot be damaged. Three of the twelve blocking codes
// stop being reachable rather than merely unlikely.
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
func (s *Service) auditGenerateFailure(ctx context.Context, ws identity.Workspace, task string, attempt int, meta map[string]any) {
	meta["attempts"] = attempt
	meta["task_description_chars"] = len([]rune(task))
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
