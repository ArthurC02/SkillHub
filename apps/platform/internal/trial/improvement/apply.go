package eval

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

const (
	BlockedPathOutOfBounds  = "path_out_of_bounds"
	BlockedTargetChanged    = "target_changed"
	BlockedValidation       = "validation_blocked"
	BlockedAccessRestricted = "access_restricted"
	BlockedDiffUnavailable  = "diff_unavailable"
)

const maxTargetFileBytes = 1 << 20

const (
	diffContextLines = 3
	binaryProbeBytes = 8000
)

type Blocked struct {
	SuggestionID string `json:"suggestion_id"`
	Reason       string `json:"blocked_reason"`
	Message      string `json:"message"`
}

var ErrNotAccepted = errors.New("every suggestion must be accepted before it can be applied")

var errNoStore = errors.New("no object store is configured, so package contents cannot be read")

type suggestionCtx struct {
	suggestion gen.EvaluationSuggestion
	skill      SkillFacts
	origin     VersionFacts
	latest     VersionFacts
	latestZip  []byte
	originFS   fs.FS
	latestFS   fs.FS
}

func (s *Service) loadSuggestion(
	ctx context.Context, workspaceID, id pgtype.UUID,
) (suggestionCtx, error) {
	var sc suggestionCtx
	if s.ReadVersion == nil || s.ReadSkill == nil || s.ReadLatestVersion == nil {
		return sc, errRegistryReadNotConfigured
	}
	q := s.queries()

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

func (s *Service) loadVersions(
	ctx context.Context, workspaceID pgtype.UUID, ev gen.Evaluation, sc suggestionCtx,
) (suggestionCtx, error) {
	if s.ReadVersion == nil || s.ReadSkill == nil || s.ReadLatestVersion == nil {
		return sc, errRegistryReadNotConfigured
	}
	run, err := s.runFacts(ctx, workspaceID, ev.RunID)
	if err != nil {
		return sc, err
	}
	if sc.origin, err = s.existingVersion(ctx, workspaceID, run.SkillVersionID); err != nil {
		return sc, err
	}
	var found bool
	if sc.skill, found, err = s.ReadSkill(ctx, workspaceID, sc.origin.SkillID); !found && err == nil {
		return sc, ErrNotFound
	} else if err != nil {
		return sc, err
	}

	if sc.latest, found, err = s.ReadLatestVersion(ctx, workspaceID, sc.skill.ID); !found && err == nil {
		return sc, ErrNotFound
	} else if err != nil {
		return sc, err
	}

	var originData []byte
	if sc.originFS, originData, err = s.readPackage(ctx, sc.origin.stored()); err != nil {
		return sc, err
	}

	if sc.origin.ID == sc.latest.ID {
		sc.latestFS, sc.latestZip = sc.originFS, originData
		return sc, nil
	}
	sc.latestFS, sc.latestZip, err = s.readPackage(ctx, sc.latest.stored())
	return sc, err
}

func (s *Service) existingVersion(ctx context.Context, workspaceID, versionID pgtype.UUID) (VersionFacts, error) {
	version, found, err := s.ReadVersion(ctx, workspaceID, versionID)
	if !found && err == nil {
		return version, ErrNotFound
	}
	return version, err
}

func (s *Service) store() ObjectStore { return s.Store }

func (s *Service) readPackage(ctx context.Context, stored skillpkg.StoredSkill) (fs.FS, []byte, error) {
	if s.store() == nil {
		return nil, nil, errNoStore
	}
	data, err := s.store().Get(ctx, stored.ObjectKey)
	if err != nil {
		return nil, nil, err
	}
	fsys, err := stored.Open(data)
	if err != nil {
		return nil, nil, err
	}
	return fsys, data, nil
}

func check(sc suggestionCtx) (string, *Blocked) {
	sug := sc.suggestion
	id := pgconv.UUIDString(sug.ID)
	block := func(reason, msg string) (string, *Blocked) {
		return "", &Blocked{SuggestionID: id, Reason: reason, Message: msg}
	}

	if sc.skill.AccessRestricted {
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
		FromFile: target, ToFile: target, Context: diffContextLines,
	})
	if err != nil || diff == "" {
		return block(BlockedDiffUnavailable,
			"no difference could be rendered for this suggestion, and EVAL-002 does not allow "+
				"applying a change the user could not see")
	}
	return diff, nil
}

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
	if bytes.IndexByte(data[:min(len(data), binaryProbeBytes)], 0) >= 0 {
		return "", errors.New("it is not text, so there is no diff a user could approve")
	}
	return string(data), nil
}

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

type Diff struct {
	TargetPath    string `json:"target_path"`
	UnifiedDiff   string `json:"unified_diff,omitempty"`
	Applicable    bool   `json:"applicable"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	Message       string `json:"message,omitempty"`
}

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
		patched, err := rewriteArchiveEntry(zw, f, root, patches)
		if err != nil {
			return nil, err
		}
		if patched {
			written[strings.TrimPrefix(f.Name, root)] = true
		}
	}

	if err := addNewArchiveEntries(zw, root, patches, written); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func rewriteArchiveEntry(zw *zip.Writer, f *zip.File, root string, patches map[string]string) (patched bool, err error) {
	header := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified}
	if strings.HasSuffix(f.Name, "/") {
		header.Method = zip.Store
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return false, err
	}
	if content, replaced := patches[strings.TrimPrefix(f.Name, root)]; replaced &&
		!strings.HasSuffix(f.Name, "/") {
		if _, err := io.WriteString(w, content); err != nil {
			return true, err
		}
		return true, nil
	}
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	_, err = io.Copy(w, rc) //nolint:gosec // bounded by PackageFS's unpacked-size check
	rc.Close()
	return false, err
}

func addNewArchiveEntries(zw *zip.Writer, root string, patches map[string]string, written map[string]bool) error {
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
			return err
		}
		if _, err := io.WriteString(w, patches[p]); err != nil {
			return err
		}
	}
	return nil
}

type ApplyResult struct {
	Created     bool
	Version     ingest.UploadResult
	Applied     []string
	Rejected    []Blocked
	NotAccepted []string
}

func (s *Service) ApplySuggestions(
	ctx context.Context, ws identity.Workspace, skillID, evaluationID pgtype.UUID, ids []pgtype.UUID,
) (ApplyResult, error) {
	var out ApplyResult
	if s.Versions == nil || s.store() == nil {
		return out, errNoStore
	}
	base, err := s.evaluatedSkill(ctx, ws.ID, skillID, evaluationID)
	if err != nil {
		return out, err
	}
	suggestions, notAccepted, err := s.selectedSuggestions(ctx, ws.ID, evaluationID, ids)
	out.NotAccepted = notAccepted
	if err != nil {
		return out, err
	}
	if len(out.NotAccepted) > 0 {
		return out, ErrNotAccepted
	}
	sortByTarget(suggestions)

	plan := planPatches(base, suggestions, len(ids))
	out.Applied, out.Rejected = plan.appliedIDs, plan.rejected
	if len(plan.patches) == 0 {
		return out, nil
	}
	if blocked := validatePatched(base, plan.patches, out.Applied); blocked != nil {
		out.rejectApplied(blocked.Reason, blocked.Message)
		return out, nil
	}
	return s.buildImprovedVersion(ctx, evaluatedSkillRef{ws: ws, skillID: skillID, evaluationID: evaluationID}, base, plan, out)
}

func (s *Service) evaluatedSkill(ctx context.Context, workspaceID, skillID, evaluationID pgtype.UUID) (suggestionCtx, error) {
	ev, err := s.queries().GetEvaluation(ctx, gen.GetEvaluationParams{
		ID: evaluationID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return suggestionCtx{}, ErrNotFound
	}
	if err != nil {
		return suggestionCtx{}, err
	}
	base, err := s.loadVersions(ctx, workspaceID, ev, suggestionCtx{})
	if err != nil {
		return suggestionCtx{}, err
	}
	if base.skill.ID != skillID {
		return suggestionCtx{}, ErrNotFound
	}
	return base, nil
}

func (s *Service) selectedSuggestions(
	ctx context.Context, workspaceID, evaluationID pgtype.UUID, ids []pgtype.UUID,
) (suggestions []gen.EvaluationSuggestion, notAccepted []string, err error) {
	suggestions = make([]gen.EvaluationSuggestion, 0, len(ids))
	seenIDs := make(map[string]bool, len(ids))
	for _, id := range ids {
		idText := pgconv.UUIDString(id)
		if seenIDs[idText] {
			continue
		}
		seenIDs[idText] = true
		sug, err := s.queries().GetEvaluationSuggestion(ctx, gen.GetEvaluationSuggestionParams{
			ID: id, WorkspaceID: workspaceID,
		})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && sug.EvaluationID != evaluationID) {
			return nil, notAccepted, ErrNotFound
		}
		if err != nil {
			return nil, notAccepted, err
		}
		if Decision(sug.Decision) != DecisionAccepted {
			notAccepted = append(notAccepted, pgconv.UUIDString(sug.ID))
		}
		suggestions = append(suggestions, sug)
	}
	return suggestions, notAccepted, nil
}

func sortByTarget(suggestions []gen.EvaluationSuggestion) {
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
}

type patchPlan struct {
	patches    map[string]string
	applied    []pgtype.UUID
	appliedIDs []string
	rejected   []Blocked
}

func planPatches(base suggestionCtx, suggestions []gen.EvaluationSuggestion, capacity int) patchPlan {
	plan := patchPlan{patches: map[string]string{}, applied: make([]pgtype.UUID, 0, capacity)}
	targetCounts := make(map[string]int, len(suggestions))
	for _, sug := range suggestions {
		if target, ok := cleanTargetPath(sug.TargetPath); ok {
			targetCounts[target]++
		}
	}
	for _, sug := range suggestions {
		target, _ := cleanTargetPath(sug.TargetPath)
		if targetCounts[target] > 1 {
			plan.rejected = append(plan.rejected, Blocked{
				SuggestionID: pgconv.UUIDString(sug.ID), Reason: BlockedTargetChanged,
				Message: "multiple suggestions replace " + target + "; select exactly one",
			})
			continue
		}
		sc := base
		sc.suggestion = sug
		if _, blocked := check(sc); blocked != nil {
			plan.rejected = append(plan.rejected, *blocked)
			continue
		}
		plan.patches[target] = sug.ProposedContent
		plan.applied = append(plan.applied, sug.ID)
		plan.appliedIDs = append(plan.appliedIDs, pgconv.UUIDString(sug.ID))
	}
	return plan
}

func (r *ApplyResult) rejectApplied(reason, message string) {
	for _, id := range r.Applied {
		r.Rejected = append(r.Rejected, Blocked{SuggestionID: id, Reason: reason, Message: message})
	}
	r.Applied = nil
}

type evaluatedSkillRef struct {
	ws           identity.Workspace
	skillID      pgtype.UUID
	evaluationID pgtype.UUID
}

func (s *Service) buildImprovedVersion(
	ctx context.Context, target evaluatedSkillRef, base suggestionCtx, plan patchPlan, out ApplyResult,
) (ApplyResult, error) {
	ws, skillID, evaluationID := target.ws, target.skillID, target.evaluationID
	patched, err := patchArchive(base.latestZip, plan.patches)
	if err != nil {
		return out, err
	}
	res, err := s.Versions.SaveImprovedVersion(ctx, ws, skillID, patched, ingest.Improvement{
		EvaluationID: evaluationID, SuggestionIDs: plan.applied, BaseVersionID: base.latest.ID,
	})
	if errors.Is(err, ingest.ErrBaseVersionMoved) {
		out.rejectApplied(BlockedTargetChanged, "a newer version of this skill was saved while these changes were being applied, "+
			"so they were made to an older package; open the newest version and apply them again")
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if res.Report.Blocked {
		out.rejectApplied(BlockedValidation, "with these changes applied the package no longer passes import validation")
		return out, nil
	}
	if res.Duplicate {
		if err := s.RecordSuggestionsApplied(ctx, ws.ID, evaluationID, res.Version.ID, plan.applied); err != nil {
			return out, err
		}
	}
	out.Created, out.Version = true, ingest.NewUploadResult(res)
	return out, nil
}
