package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

const MaxSkillsPerImport = 50

var ErrTooManySkills = errors.New("ingest: source holds more skills than one import may create")

type plannedSkill struct {
	path string
	pkg  preparedPackage
}

type importPlan struct {
	shape     skillpkg.SourceShape
	plugin    *skillpkg.PluginFacts
	excluded  []skillpkg.Finding
	objectKey string

	admitted []plannedSkill
	refused  []plannedSkill
}

func (p importPlan) blocked() bool { return len(p.admitted) == 0 }

func planImport(data []byte) (importPlan, error) {
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return importPlan{}, err
	}
	d := skillpkg.Discover(fsys)
	plan := importPlan{shape: d.Shape, plugin: d.Plugin, excluded: d.Excluded}
	if d.Blocked {
		plan.refused = []plannedSkill{{pkg: preparedPackage{
			report: skillpkg.Report{Findings: d.Findings, Blocked: true}}}}
		return plan, nil
	}
	if len(d.Skills) > MaxSkillsPerImport {
		return importPlan{}, fmt.Errorf("%w: %d found, %d allowed",
			ErrTooManySkills, len(d.Skills), MaxSkillsPerImport)
	}

	sum := sha256.Sum256(data)
	packageHash := hex.EncodeToString(sum[:])
	plan.objectKey = "packages/" + packageHash + ".zip"

	for _, dir := range d.Skills {
		pkg, err := prepareSkillAt(fsys, dir, plan.objectKey, packageHash, d.Findings)
		if err != nil {
			return importPlan{}, err
		}
		planned := plannedSkill{path: dir, pkg: pkg}
		if pkg.report.Blocked {
			plan.refused = append(plan.refused, planned)
			continue
		}
		plan.admitted = append(plan.admitted, planned)
	}
	plan.refuseRepeatedNames()
	return plan, nil
}

// Two directories carrying the same manifest name would otherwise become one
// Skill and its second version, silently merging two different Skills.
func (p *importPlan) refuseRepeatedNames() {
	admitted := p.admitted
	p.admitted = nil
	claimedBy := make(map[string]string, len(admitted))
	for _, planned := range admitted {
		name := planned.pkg.report.Manifest.Name
		first, taken := claimedBy[name]
		if !taken {
			claimedBy[name] = planned.path
			p.admitted = append(p.admitted, planned)
			continue
		}
		planned.pkg.report = withSourceFindings(planned.pkg.report, []skillpkg.Finding{{
			Severity: skillpkg.SeverityError, Code: skillpkg.CodeDuplicateSkillName, Path: planned.path,
			Message: "這個來源裡的 " + first + " 已經用了 name " + name +
				"。一個名字在一次匯入裡只能建立一個 Skill——否則第二個會變成第一個的新版本，把兩個不同的 Skill 併成一個。",
		}})
		p.refused = append(p.refused, planned)
	}
}

// A skill that IS the package keeps the package digest it has always had, so
// content already imported stays recognisable as the same content; a skill that
// is one directory inside a larger source is hashed over its own files.
func prepareSkillAt(fsys fs.FS, dir, objectKey, packageHash string, sourceFindings []skillpkg.Finding) (preparedPackage, error) {
	sub := fsys
	if dir != "." {
		var err error
		if sub, err = fs.Sub(fsys, dir); err != nil {
			return preparedPackage{}, err
		}
	}
	p := preparedPackage{report: skillpkg.Validate(sub), objectKey: objectKey}
	if dir != "." {
		p.sourcePath = dir
	}
	if dir != "." {
		p.report = withSourceFindings(p.report, archiveFindings(fsys), sourceFindings)
	}
	if p.report.Blocked {
		return p, nil
	}
	p.contentHash = packageHash
	if dir != "." {
		hash, err := skillpkg.SubtreeHash(sub)
		if err != nil {
			return preparedPackage{}, err
		}
		p.contentHash = hash
	}
	if md, err := fs.ReadFile(sub, "SKILL.md"); err == nil {
		if len(md) > maxEnrichMDBytes {
			md = md[:maxEnrichMDBytes]
		}
		p.skillMD = strings.ToValidUTF8(string(md), "")
	}
	_ = fs.WalkDir(sub, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || len(p.fileTree) >= maxEnrichFiles {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		p.fileTree = append(p.fileTree, path)
		return nil
	})
	return p, nil
}

// fs.Sub hands back a plain fs.FS, so findings the archive reader raised about
// the source as a whole would vanish for every skill below its root.
func archiveFindings(fsys fs.FS) []skillpkg.Finding {
	source, ok := fsys.(skillpkg.ArchiveSource)
	if !ok {
		return nil
	}
	return source.ArchiveFindings()
}

func withSourceFindings(r skillpkg.Report, groups ...[]skillpkg.Finding) skillpkg.Report {
	for _, group := range groups {
		for _, f := range group {
			r.Findings = append(r.Findings, f)
			if f.Severity == skillpkg.SeverityError {
				r.Blocked = true
			}
		}
	}
	return r
}

type Refusal struct {
	Path   string
	Report skillpkg.Report
}

type SkillImport struct {
	Path string
	Result
}

type SourceResult struct {
	Shape    skillpkg.SourceShape
	Plugin   *skillpkg.PluginFacts
	Excluded []skillpkg.Finding

	Imported []SkillImport
	Refused  []Refusal

	Report skillpkg.Report
}

func (r SourceResult) Blocked() bool { return len(r.Imported) == 0 }

func (s *Service) importSource(ctx context.Context, ws identity.Workspace, data []byte, src sourceMeta) (SourceResult, error) {
	plan, err := planImport(data)
	if err != nil {
		return SourceResult{}, err
	}
	out := SourceResult{Shape: plan.shape, Plugin: plan.plugin, Excluded: plan.excluded}
	src.Plugin = plan.plugin
	for _, refused := range plan.refused {
		out.Refused = append(out.Refused, Refusal{Path: refused.pkg.sourcePath, Report: refused.pkg.report})
	}
	if plan.blocked() {
		out.Report = sourceLevelReport(plan)
		return out, nil
	}

	enriched := make([]enrichment, len(plan.admitted))
	for i, planned := range plan.admitted {
		enriched[i] = s.enrichPackage(ctx, planned.pkg, ws.ID)
	}

	tx, release, err := s.beginPackageWrite(ctx, ws, plan.objectKey, data)
	if err != nil {
		return SourceResult{}, err
	}
	defer release()
	for i, planned := range plan.admitted {
		held, err := nameHeldByAnotherSource(ctx, tx, ws, planned.pkg, src)
		if err != nil {
			return SourceResult{}, err
		}
		if held != nil {
			report := withSourceFindings(planned.pkg.report, []skillpkg.Finding{*held})
			out.Refused = append(out.Refused, Refusal{Path: planned.pkg.sourcePath, Report: report})
			continue
		}
		res, err := s.importOne(ctx, tx, ws, planned.pkg, src, enriched[i])
		if err != nil {
			return SourceResult{}, err
		}
		out.Imported = append(out.Imported, SkillImport{Path: planned.pkg.sourcePath, Result: res})
	}
	if out.Blocked() {
		out.Report = skillpkg.Report{Blocked: true}
		if len(out.Refused) == 1 {
			out.Report = out.Refused[0].Report
		}
	}
	return out, tx.Commit(ctx)
}

func nameHeldByAnotherSource(
	ctx context.Context, tx pgx.Tx, ws identity.Workspace, p preparedPackage, src sourceMeta,
) (*skillpkg.Finding, error) {
	incoming := originOf(pluginNameOf(src.Plugin), src.URL)
	if incoming == "" || src.Type == SourceGenerated {
		return nil, nil
	}
	name := p.report.Manifest.Name
	root, found, err := registry.LoadSkillNamed(ctx, tx, ws.ID, name)
	if err != nil || !found {
		return nil, err
	}
	latest, found, err := registry.LatestVersionIn(ctx, tx, ws.ID, root.Skill().ID)
	if err != nil || !found || !latest.SourceID.Valid {
		return nil, err
	}
	source, err := gen.New(tx).GetSkillSource(ctx, gen.GetSkillSourceParams{ID: latest.SourceID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	held := originOf(source.PluginName, source.SourceUrl)
	if held == "" || held == incoming {
		return nil, nil
	}
	return &skillpkg.Finding{
		Severity: skillpkg.SeverityError, Code: skillpkg.CodeNameHeldByAnotherSource, Path: p.sourcePath,
		Message: "name " + name + " 已經屬於你工作區裡另一個來源（" + held + "）的 Skill。" +
			"匯入不會把兩個不同來源的 Skill 併成一個。要把這一份當成那個 Skill 的新版本，" +
			"到那個 Skill 的頁面用「上傳新版本」；要兩個都留，改掉這一份 SKILL.md 的 name 再匯入。",
	}, nil
}

func pluginNameOf(p *skillpkg.PluginFacts) *string {
	if p == nil {
		return nil
	}
	return &p.Name
}

func originOf(pluginName, sourceURL *string) string {
	if pluginName != nil && *pluginName != "" {
		return "Plugin " + *pluginName
	}
	if sourceURL != nil && *sourceURL != "" {
		return *sourceURL
	}
	return ""
}

// A source that yielded nothing has one report to show: the whole-source
// refusal when no skill was found, otherwise the findings of the only skill.
func sourceLevelReport(plan importPlan) skillpkg.Report {
	if len(plan.refused) == 1 {
		return plan.refused[0].pkg.report
	}
	return skillpkg.Report{Blocked: true}
}

func (s *Service) importOne(
	ctx context.Context, tx pgx.Tx, ws identity.Workspace,
	p preparedPackage, src sourceMeta, e enrichment,
) (Result, error) {
	res := Result{Report: p.report}
	root, found, err := registry.LoadSkillNamed(ctx, tx, ws.ID, p.report.Manifest.Name)
	if err != nil {
		return Result{}, err
	}
	if found && src.Type == SourceGenerated {
		return Result{}, fmt.Errorf("%w: %q", ErrGeneratedNameCollision, root.Skill().Name)
	}
	if !found {
		if root, err = registry.SkillFromPackage(ws.ID, p.report, redistributionFor(ws, src)); err != nil {
			return Result{}, err
		}
		if err := registry.SaveSkill(ctx, tx, root); err != nil {
			return Result{}, err
		}
	}
	res.Skill = root.Skill()

	res.Version, res.Duplicate, err = s.persistVersion(ctx, tx, ws, root, p, src, e)
	if err != nil {
		return Result{}, err
	}
	importMeta := map[string]any{"source_type": string(src.Type)}
	usageMeta(importMeta, src.CostUSD, src.PromptTokens, src.CompletionTokens)
	if err := auditVersion(ctx, tx, ws, audit.ActionSkillImport, res, importMeta); err != nil {
		return Result{}, err
	}
	return res, nil
}
