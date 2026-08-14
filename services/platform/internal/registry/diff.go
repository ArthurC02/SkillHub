package registry

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pmezard/go-difflib/difflib"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// FileDiff is one file's change between two versions (WS-003).
type FileDiff struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added | removed | modified
	// Diff is a unified diff for text files; empty for binary or oversized
	// files, where the status line is the whole story.
	Diff string `json:"diff,omitempty"`
}

// maxDiffFileBytes caps per-file text diffing.
// ponytail: same flat-cap style as skillpkg's scan limit.
const maxDiffFileBytes = 1 << 20 // 1 MiB

// DiffVersions compares two versions of the same skill, both scoped to ws.
func (s *Service) DiffVersions(ctx context.Context, ws gen.Workspace, skillID, fromID, toID pgtype.UUID) ([]FileDiff, error) {
	q := gen.New(s.Pool)
	load := func(versionID pgtype.UUID) (fs.FS, error) {
		v, err := q.GetSkillVersion(ctx, gen.GetSkillVersionParams{ID: versionID, WorkspaceID: ws.ID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && v.SkillID != skillID) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		data, err := s.Store.Get(ctx, v.PackageObjectKey)
		if err != nil {
			return nil, err
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("stored package unreadable: %w", err)
		}
		return zr, nil
	}

	fromFS, err := load(fromID)
	if err != nil {
		return nil, err
	}
	toFS, err := load(toID)
	if err != nil {
		return nil, err
	}
	return diffFS(fromFS, toFS)
}

// diffFS walks both trees and reports added, removed, and modified files,
// with unified diffs for readable text.
func diffFS(from, to fs.FS) ([]FileDiff, error) {
	fromFiles, err := listFiles(from)
	if err != nil {
		return nil, err
	}
	toFiles, err := listFiles(to)
	if err != nil {
		return nil, err
	}

	paths := map[string]bool{}
	for p := range fromFiles {
		paths[p] = true
	}
	for p := range toFiles {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	diffs := make([]FileDiff, 0)
	for _, p := range sorted {
		inFrom, inTo := fromFiles[p], toFiles[p]
		switch {
		case inFrom && !inTo:
			diffs = append(diffs, FileDiff{Path: p, Status: "removed"})
		case !inFrom && inTo:
			diffs = append(diffs, FileDiff{Path: p, Status: "added"})
		default:
			a, errA := fs.ReadFile(from, p)
			b, errB := fs.ReadFile(to, p)
			if errA != nil || errB != nil {
				return nil, errors.Join(errA, errB)
			}
			if bytes.Equal(a, b) {
				continue
			}
			diffs = append(diffs, FileDiff{Path: p, Status: "modified", Diff: textDiff(p, a, b)})
		}
	}
	return diffs, nil
}

func listFiles(fsys fs.FS) (map[string]bool, error) {
	files := map[string]bool{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files[path] = true
		}
		return nil
	})
	return files, err
}

func textDiff(path string, a, b []byte) string {
	if len(a) > maxDiffFileBytes || len(b) > maxDiffFileBytes || isBinary(a) || isBinary(b) {
		return ""
	}
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(a)), B: difflib.SplitLines(string(b)),
		FromFile: path, ToFile: path, Context: 3,
	})
	if err != nil {
		return ""
	}
	return text
}

func isBinary(data []byte) bool {
	n := min(len(data), 8000)
	for _, c := range data[:n] {
		if c == 0 {
			return true
		}
	}
	return false
}
