package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
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
	return plan, nil
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
