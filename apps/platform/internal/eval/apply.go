package eval

// Turning an accepted suggestion into a package change (EVAL-002 clauses 3 and 4).
//
// Two callers, one set of checks: GET /suggestions/{id}/diff previews and
// POST /skills/{id}/versions/from-suggestions applies, and both go through
// check() so a preview can never promise something the apply call then refuses
// (public.yaml SuggestionBlockedReason: "one vocabulary, served from one place").
//
// Nothing here executes any part of a package (iron rule 1): an archive is read
// as bytes, patched as bytes, and handed to the same static validation an import
// goes through. And nothing here edits a version — applying builds a NEW one
// through ingest.SaveVersion, so the version the suggestion was written against
// keeps every byte it had and the runs pointing at it still mean what they meant
// (iron rule 4).

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pmezard/go-difflib/difflib"

	"github.com/ArthurC02/skillhub/apps/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
)

// The reasons a suggestion cannot be applied (public.yaml SuggestionBlockedReason).
// Fixed vocabulary: the preview and the apply call answer from these and nothing
// else, so the two can never disagree about what stops a change.
const (
	BlockedPathOutOfBounds  = "path_out_of_bounds"
	BlockedTargetChanged    = "target_changed"
	BlockedValidation       = "validation_blocked"
	BlockedAccessRestricted = "access_restricted"
	BlockedDiffUnavailable  = "diff_unavailable"
)

// maxTargetFileBytes caps the file a suggestion may replace. Same ceiling
// registry's diff uses, for the same reason: past it there is no readable diff to
// approve, and EVAL-002 clause 3 does not allow applying a change nobody could see.
// ponytail: flat cap, same style as the other package limits.
const maxTargetFileBytes = 1 << 20 // 1 MiB

// Blocked is one refused suggestion (public.yaml RejectedSuggestion).
type Blocked struct {
	SuggestionID string `json:"suggestion_id"`
	Reason       string `json:"blocked_reason"`
	Message      string `json:"message"`
}

// ErrNotAccepted is a request to apply a suggestion the user has not accepted.
// A client mistake rather than a property of the package, so it refuses the whole
// call and creates nothing — accepting is PUT /suggestions/{id}/decision.
var ErrNotAccepted = errors.New("every suggestion must be accepted before it can be applied")

// errNoStore is a deployment with no object store or no version writer wired in.
// A configuration fault, not a refusal about the suggestion: it is a 500, never a
// `blocked_reason`, because nothing was checked.
var errNoStore = errors.New("no object store is configured, so package contents cannot be read")

// suggestionCtx is the material one check needs: the suggestion, the version it
// was written against, the version a new one would be built from, and the skill
// carrying any licensing hold.
type suggestionCtx struct {
	suggestion gen.EvaluationSuggestion
	skill      gen.Skill
	origin     gen.SkillVersion
	latest     gen.SkillVersion
	latestZip  []byte
	originFS   fs.FS
	latestFS   fs.FS
}

// loadSuggestion resolves one suggestion and everything around it, all under the
// caller's workspace (iron rule 3). Anything not visible there is ErrNotFound,
// whichever link in the chain it was (WS-006).
func (s *Service) loadSuggestion(
	ctx context.Context, workspaceID, id pgtype.UUID,
) (suggestionCtx, error) {
	q := s.queries()
	var sc suggestionCtx

	sug, err := q.GetEvaluationSuggestion(ctx, gen.GetEvaluationSuggestionParams{
		ID: id, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	}
	if err != nil {
		return sc, err
	}
	sc.suggestion = sug

	ev, err := q.GetEvaluation(ctx, gen.GetEvaluationParams{
		ID: sug.EvaluationID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	}
	if err != nil {
		return sc, err
	}
	return s.loadVersions(ctx, workspaceID, ev, sc)
}

// loadVersions fills in the two versions and their archives.
func (s *Service) loadVersions(
	ctx context.Context, workspaceID pgtype.UUID, ev gen.Evaluation, sc suggestionCtx,
) (suggestionCtx, error) {
	q := s.queries()
	run, err := q.GetRun(ctx, gen.GetRunParams{ID: ev.RunID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	}
	if err != nil {
		return sc, err
	}
	if sc.origin, err = q.GetSkillVersion(ctx, gen.GetSkillVersionParams{
		ID: run.SkillVersionID, WorkspaceID: workspaceID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	} else if err != nil {
		return sc, err
	}
	if sc.skill, err = q.GetSkill(ctx, gen.GetSkillParams{
		ID: sc.origin.SkillID, WorkspaceID: workspaceID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	} else if err != nil {
		return sc, err
	}
	// The new version is built from the newest one, not from the evaluated one:
	// anything else would silently revert every file somebody changed in between.
	// A target file that moved in the meantime is refused as `target_changed`
	// rather than overwritten (check()).
	if sc.latest, err = q.GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: sc.skill.ID, WorkspaceID: workspaceID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	} else if err != nil {
		return sc, err
	}

	originData := []byte(nil)
	if sc.originFS, originData, err = s.readPackage(ctx, sc.origin.PackageObjectKey); err != nil {
		return sc, err
	}
	// Usually the same archive, and re-reading it would be a second network round
	// trip to compare a file with itself.
	if sc.origin.ID == sc.latest.ID {
		sc.latestFS, sc.latestZip = sc.originFS, originData
		return sc, nil
	}
	sc.latestFS, sc.latestZip, err = s.readPackage(ctx, sc.latest.PackageObjectKey)
	return sc, err
}

func (s *Service) store() ObjectStore { return s.Store }

// readPackage opens a stored archive for reading. Analysis only — the package
// root is resolved by skillpkg.PackageFS so a reader here sees exactly the tree the
// importer validated.
func (s *Service) readPackage(ctx context.Context, key string) (fs.FS, []byte, error) {
	if s.store() == nil {
		return nil, nil, errNoStore
	}
	data, err := s.store().Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return nil, nil, err
	}
	return fsys, data, nil
}

// check answers, for one suggestion, "what would this change and may it be
// applied". A non-nil *Blocked means it may not, and the diff is then whatever
// could still be shown (usually nothing).
//
// Order matters: the licensing hold is first because a held skill's contents must
// not be reproduced at all (SEC-011), and a diff is a reproduction.
func check(sc suggestionCtx) (string, *Blocked) {
	sug := sc.suggestion
	id := pgconv.UUIDString(sug.ID)
	block := func(reason, msg string) (string, *Blocked) {
		return "", &Blocked{SuggestionID: id, Reason: reason, Message: msg}
	}

	if sc.skill.AccessRestriction != nil && strings.TrimSpace(*sc.skill.AccessRestriction) != "" {
		return block(BlockedAccessRestricted,
			"this skill's materials are held back while a licensing question about them is "+
				"open, so its contents are not reproduced and no version can be built from them")
	}

	target, ok := cleanTargetPath(sug.TargetPath)
	if !ok {
		return block(BlockedPathOutOfBounds,
			"the suggestion names a path that does not resolve inside the package, "+
				"so nothing it proposes can be written")
	}
	origin, originErr := readTarget(sc.originFS, target)
	if originErr != nil {
		return block(BlockedDiffUnavailable, "the file this suggestion changes could not be "+
			"read from the version it was written against: "+originErr.Error())
	}
	current, currentErr := readTarget(sc.latestFS, target)
	if currentErr != nil {
		return block(BlockedDiffUnavailable, "the file this suggestion changes could not be "+
			"read from the newest version of this skill: "+currentErr.Error())
	}
	if origin != current {
		return block(BlockedTargetChanged, fmt.Sprintf(
			"%s is no longer what it was when this suggestion was written, so the change was "+
				"reasoned about other bytes; re-evaluate the newest version to get a suggestion "+
				"about it", target))
	}
	if sug.ProposedContent == origin {
		return block(BlockedDiffUnavailable,
			"the proposed content is identical to what is already in the package, "+
				"so there is nothing to show and nothing to apply")
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(origin), B: difflib.SplitLines(sug.ProposedContent),
		FromFile: target, ToFile: target, Context: 3,
	})
	if err != nil || diff == "" {
		return block(BlockedDiffUnavailable,
			"no difference could be rendered for this suggestion, and EVAL-002 does not allow "+
				"applying a change the user could not see")
	}
	return diff, nil
}

// readTarget reads one package file as text. A missing file reads as empty: a
// suggestion may add a file, and "absent in both versions" is a consistent state
// rather than a change underneath it.
func readTarget(fsys fs.FS, target string) (string, error) {
	data, err := fs.ReadFile(fsys, target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("it could not be read from the archive")
	}
	if len(data) > maxTargetFileBytes {
		return "", fmt.Errorf("it is larger than the %d byte limit for a reviewable diff", maxTargetFileBytes)
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return "", errors.New("it is not text, so there is no diff a user could approve")
	}
	return string(data), nil
}

// cleanTargetPath normalises a package-relative path and reports whether it stays
// inside the package. Refused rather than sanitised: a path that had to be
// repaired is not a path the model meant, and 0024's CHECK is the floor under
// this, not a replacement for it.
func cleanTargetPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return "", false
	}
	cleaned := path.Clean(p)
	if cleaned != p || !fs.ValidPath(cleaned) || cleaned == "." {
		return "", false
	}
	return cleaned, true
}

// Diff is one preview (public.yaml SuggestionDiff).
type Diff struct {
	TargetPath    string `json:"target_path"`
	UnifiedDiff   string `json:"unified_diff,omitempty"`
	Applicable    bool   `json:"applicable"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	Message       string `json:"message,omitempty"`
}

// SuggestionDiff serves GET /suggestions/{id}/diff: what applying this would
// change, or the reason there is nothing to show. It runs the checks the apply
// call runs, including the static validation of the patched package, so
// `applicable: false` here and a rejection there carry the same reason.
func (s *Service) SuggestionDiff(ctx context.Context, workspaceID, id pgtype.UUID) (Diff, error) {
	sc, err := s.loadSuggestion(ctx, workspaceID, id)
	if errors.Is(err, ErrNotFound) {
		return Diff{}, ErrNotFound
	}
	if err != nil {
		return Diff{}, err
	}
	target, _ := cleanTargetPath(sc.suggestion.TargetPath)
	out := Diff{TargetPath: sc.suggestion.TargetPath}
	if target != "" {
		out.TargetPath = target
	}

	diff, blocked := check(sc)
	if blocked == nil {
		// The same validation the apply call runs, run here so the preview cannot
		// promise a change that import-grade validation would refuse.
		blocked = validatePatched(sc, map[string]string{target: sc.suggestion.ProposedContent},
			[]string{pgconv.UUIDString(sc.suggestion.ID)})
	}
	if blocked != nil {
		out.Applicable, out.BlockedReason, out.Message = false, blocked.Reason, blocked.Message
		return out, nil
	}
	out.Applicable, out.UnifiedDiff = true, diff
	return out, nil
}

// validatePatched patches the newest archive and re-runs the same static
// validation an import goes through (evaluation-design §5.2). A blocking finding
// refuses every suggestion in the set: the patched package is one package, and
// which of several changes broke it is not something this can attribute.
func validatePatched(sc suggestionCtx, patches map[string]string, ids []string) *Blocked {
	patched, err := patchArchive(sc.latestZip, patches)
	if err != nil {
		return &Blocked{SuggestionID: first(ids), Reason: BlockedDiffUnavailable,
			Message: "the stored package could not be rewritten with this change: " + err.Error()}
	}
	fsys, err := skillpkg.PackageFS(patched)
	if err != nil {
		return &Blocked{SuggestionID: first(ids), Reason: BlockedValidation,
			Message: "the package would no longer be readable with this change applied"}
	}
	report := skillpkg.Validate(fsys)
	if !report.Blocked {
		return nil
	}
	codes := map[string]struct{}{}
	for _, f := range report.Findings {
		if f.Severity == skillpkg.SeverityError {
			codes[f.Code] = struct{}{}
		}
	}
	list := make([]string, 0, len(codes))
	for c := range codes {
		list = append(list, c)
	}
	sort.Strings(list)
	// The codes, not the messages: a message can quote package content and this
	// string reaches an HTTP response.
	return &Blocked{SuggestionID: first(ids), Reason: BlockedValidation,
		Message: "with this change applied the package no longer passes the validation an " +
			"import has to pass (" + strings.Join(list, ", ") + ")"}
}

func first(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// patchArchive rewrites one zip with some files replaced, keeping every other
// entry byte for byte. The package root is ingest's, not a second rule of its own:
// a package that lives in a single top-level directory has to be patched inside
// that directory, and getting that wrong writes the new file next to the package.
func patchArchive(data []byte, patches map[string]string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("stored package unreadable: %w", err)
	}
	root := skillpkg.PackageRoot(zr)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	written := map[string]bool{}
	for _, f := range zr.File {
		header := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified}
		if strings.HasSuffix(f.Name, "/") {
			header.Method = zip.Store
		}
		w, err := zw.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if content, replaced := patches[strings.TrimPrefix(f.Name, root)]; replaced &&
			!strings.HasSuffix(f.Name, "/") {
			written[strings.TrimPrefix(f.Name, root)] = true
			if _, err := io.WriteString(w, content); err != nil {
				return nil, err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(w, rc) //nolint:gosec // bounded by PackageFS's unpacked-size check
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	// Files the package did not have yet, in path order so the archive is
	// reproducible for identical input (INGEST-005 dedupes on the content hash).
	added := make([]string, 0, len(patches))
	for p := range patches {
		if !written[p] {
			added = append(added, p)
		}
	}
	sort.Strings(added)
	for _, p := range added {
		w, err := zw.Create(root + p)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(w, patches[p]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ApplyResult is the answer to POST /skills/{id}/versions/from-suggestions.
// Created false means no version was built and nothing was written.
type ApplyResult struct {
	Created     bool
	Version     ingest.UploadResult
	Applied     []string
	Rejected    []Blocked
	NotAccepted []string
}

// ApplySuggestions turns the accepted suggestions of one evaluation into exactly
// one new Skill Version (EVAL-002 clause 4, iron rule 4).
//
// It writes through ingest.SaveVersion — the same path an upload takes — so the
// patched bytes go through static validation, land in object storage, get their
// version row, their license provenance, their search projection and their audit
// entry. Nothing about a suggestion-built version is exempt from what an uploaded
// one goes through, and there is no second writer to keep in step.
func (s *Service) ApplySuggestions(
	ctx context.Context, ws gen.Workspace, skillID, evaluationID pgtype.UUID, ids []pgtype.UUID,
) (ApplyResult, error) {
	var out ApplyResult
	if s.Versions == nil || s.store() == nil {
		return out, errNoStore
	}

	ev, err := s.queries().GetEvaluation(ctx, gen.GetEvaluationParams{
		ID: evaluationID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	base, err := s.loadVersions(ctx, ws.ID, ev, suggestionCtx{})
	if err != nil {
		return out, err
	}
	// The evaluation has to be about this skill. Without this, an evaluation of one
	// skill could seed a version of another one.
	if base.skill.ID != skillID {
		return out, ErrNotFound
	}

	// Everything is resolved before anything is checked, so a request naming one
	// undecided suggestion is refused whole instead of half-applied.
	suggestions := make([]gen.EvaluationSuggestion, 0, len(ids))
	seenIDs := make(map[string]bool, len(ids))
	for _, id := range ids {
		idText := pgconv.UUIDString(id)
		if seenIDs[idText] {
			continue
		}
		seenIDs[idText] = true
		sug, err := s.queries().GetEvaluationSuggestion(ctx, gen.GetEvaluationSuggestionParams{
			ID: id, WorkspaceID: ws.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && sug.EvaluationID != evaluationID) {
			// A suggestion from another evaluation is not a rejection with a reason,
			// it is a request about something that is not there (contract: 404).
			return out, ErrNotFound
		}
		if err != nil {
			return out, err
		}
		if sug.Decision != DecisionAccepted {
			out.NotAccepted = append(out.NotAccepted, pgconv.UUIDString(sug.ID))
		}
		suggestions = append(suggestions, sug)
	}
	if len(out.NotAccepted) > 0 {
		// Not a `blocked_reason`: nothing about the package stops these, the user
		// simply has not said yes. Accepting is PUT /suggestions/{id}/decision.
		return out, ErrNotAccepted
	}
	sort.Slice(suggestions, func(i, j int) bool {
		left, leftOK := cleanTargetPath(suggestions[i].TargetPath)
		right, rightOK := cleanTargetPath(suggestions[j].TargetPath)
		if !leftOK {
			left = suggestions[i].TargetPath
		}
		if !rightOK {
			right = suggestions[j].TargetPath
		}
		if left != right {
			return left < right
		}
		return pgconv.UUIDString(suggestions[i].ID) < pgconv.UUIDString(suggestions[j].ID)
	})

	patches := map[string]string{}
	targetCounts := make(map[string]int, len(suggestions))
	for _, sug := range suggestions {
		if target, ok := cleanTargetPath(sug.TargetPath); ok {
			targetCounts[target]++
		}
	}
	applied := make([]pgtype.UUID, 0, len(ids))
	for _, sug := range suggestions {
		target, _ := cleanTargetPath(sug.TargetPath)
		if targetCounts[target] > 1 {
			out.Rejected = append(out.Rejected, Blocked{
				SuggestionID: pgconv.UUIDString(sug.ID), Reason: BlockedTargetChanged,
				Message: "multiple suggestions replace " + target + "; select exactly one",
			})
			continue
		}
		sc := base
		sc.suggestion = sug
		_, blocked := check(sc)
		if blocked != nil {
			out.Rejected = append(out.Rejected, *blocked)
			continue
		}
		patches[target] = sug.ProposedContent
		applied = append(applied, sug.ID)
		out.Applied = append(out.Applied, pgconv.UUIDString(sug.ID))
	}
	if len(patches) == 0 {
		return out, nil // nothing to apply: no version is created (contract 422)
	}

	if blocked := validatePatched(base, patches, out.Applied); blocked != nil {
		// One package, one verdict: every suggestion in the set comes back rejected
		// with the same reason rather than one of them being blamed.
		for _, id := range out.Applied {
			out.Rejected = append(out.Rejected, Blocked{
				SuggestionID: id, Reason: blocked.Reason, Message: blocked.Message,
			})
		}
		out.Applied = nil
		return out, nil
	}

	patched, err := patchArchive(base.latestZip, patches)
	if err != nil {
		return out, err
	}
	res, err := s.Versions.SaveVersion(ctx, ws, skillID, patched)
	if err != nil {
		return out, err
	}
	if res.Report.Blocked {
		// validatePatched already ran the same check, so reaching here means the two
		// disagreed. Report it as the refusal it is rather than as a version.
		for _, id := range out.Applied {
			out.Rejected = append(out.Rejected, Blocked{SuggestionID: id, Reason: BlockedValidation,
				Message: "with these changes applied the package no longer passes import validation"})
		}
		out.Applied = nil
		return out, nil
	}

	// EVAL-002 clause 4: each applied suggestion points at the new version. A
	// separate statement from SaveVersion's transaction, so a failure here leaves a
	// version whose provenance is not recorded; re-applying is safe because
	// identical content returns the same version (INGEST-005) and re-runs this.
	if _, err := s.queries().MarkSuggestionsApplied(ctx, gen.MarkSuggestionsAppliedParams{
		SkillVersionID: res.Version.ID, Ids: applied, WorkspaceID: ws.ID,
	}); err != nil {
		return out, err
	}
	out.Created, out.Version = true, ingest.NewUploadResult(res)
	return out, nil
}
