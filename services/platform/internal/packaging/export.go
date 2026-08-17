package packaging

// Building the bytes: what may travel, how the zip is written so two builds of
// the same content are the same file, and the two hashes that answer two
// different questions (ADR-027 decisions 1 and 2).
//
// Nothing here executes anything (iron rule 1). A package is read as an fs.FS,
// filtered as bytes, and written as a zip; there is no os/exec, no unpacking to
// disk, and no interpretation of what any file means.

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"testing/fstest"
	"time"
)

// ManifestFile and InstallFile are the two files the platform adds to the root
// of every package. The third addition is the optional test-cases/ tree.
//
// Three, and their names are recognisable as the platform's. Each extra file is
// one more "did the author write this or did Skill Hub" question, and ADR-021
// already paid for that once: LICENSE.repo is matched case-sensitively precisely
// so it stays distinguishable from an author's own file.
const (
	ManifestFile = "skillhub-manifest.json"
	InstallFile  = "INSTALL.md"
	testCasesDir = "test-cases/"
)

// deflateLevel is pinned rather than left to the standard library's default.
// ADR-027 decision 2 makes canonical zip writing the packager's obligation, and
// a compression level that moves with a Go release is exactly the kind of
// silent drift that would make content_hash stop reproducing without anything
// in this repository changing.
const deflateLevel = 5

// excludedDirs never travel, at any depth. They are not Skill content, and
// .git/ in particular can carry a private remote URL and a credential cache —
// PACK-004's "internal paths" in the most literal form.
var excludedDirs = map[string]bool{
	".git": true, ".github": true, "node_modules": true,
	"__pycache__": true, ".venv": true, ".tox": true, ".mypy_cache": true,
}

// exportFile is one entry, keyed by its PACKAGE-relative path — the path inside
// the Skill root, not inside the zip. A profile may wrap the whole thing in a
// top-level directory, and manifest_hash has to mean the same thing either way.
type exportFile struct {
	path string
	data []byte
}

// collect walks the source package and returns the files that may travel.
//
// An allow-list, not a deny-list: only files that came from the Skill Version's
// own package bytes are copied, plus the platform's own additions elsewhere in
// this package. A deny-list that misses an entry is a leak; an allow-list that
// misses one is a missing file (m4/README M4-4). Everything a Run produced —
// evaluation prose, evidence excerpts, rubrics, traces, artifacts — is therefore
// absent by construction rather than by a rule that has to remember it: none of
// it was ever part of these bytes.
func collect(fsys fs.FS) ([]exportFile, error) {
	var out []exportFile
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && excludedDirs[lastSegment(path)] {
				return fs.SkipDir
			}
			return nil
		}
		if !travels(path, d) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		out = append(out, exportFile{path: path, data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// travels decides one source entry.
//
// The path rules exist here and nowhere on the import side, and that asymmetry
// is the point: the platform never unpacks to disk, so nothing it reads can
// escape anywhere. The USER unpacks to disk. This is the first and only place
// where a zip-slip check is load-bearing (packaging-design §2.1, §4.4).
func travels(path string, d fs.DirEntry) bool {
	if !fs.ValidPath(path) || path == "." || strings.ContainsAny(path, `\`) {
		return false
	}
	if strings.HasPrefix(path, "/") || hasDriveLetter(path) {
		return false
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." || seg == "." || excludedDirs[seg] {
			return false
		}
	}
	// Symlinks, devices and fifos. A zip can carry them and extraction tools
	// disagree about what to do with one, which is a decision no package gets to
	// make on a user's machine.
	if info, err := d.Info(); err != nil || !info.Mode().IsRegular() {
		return false
	}
	return true
}

func hasDriveLetter(path string) bool {
	return len(path) >= 2 && path[1] == ':' &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// addFrontmatter appends a Profile's additive frontmatter fields to SKILL.md.
//
// Additive only, and append-only within the existing block: the body is not
// touched, no existing key is rewritten, and a file whose frontmatter cannot be
// located is left exactly as it was rather than repaired. ADR-012 forbids an
// Adapter changing a Skill's task intent, and a rewrite nobody asked for is how
// that happens by accident.
func addFrontmatter(skillMD []byte, additions map[string]any) ([]byte, error) {
	if len(additions) == 0 {
		return skillMD, nil
	}
	s := string(skillMD)
	rest, ok := strings.CutPrefix(s, "---\n")
	prefix := "---\n"
	if !ok {
		if rest, ok = strings.CutPrefix(s, "---\r\n"); ok {
			prefix = "---\r\n"
		}
	}
	if !ok {
		return nil, errors.New("SKILL.md has no frontmatter block to add fields to")
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, errors.New("SKILL.md frontmatter has no closing delimiter")
	}
	keys := make([]string, 0, len(additions))
	for k := range additions {
		if reservedFrontmatterKeys[k] {
			return nil, fmt.Errorf("profile would overwrite the reserved frontmatter field %q", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var add strings.Builder
	for _, k := range keys {
		v, err := json.Marshal(additions[k])
		if err != nil {
			return nil, err
		}
		// JSON scalars are valid YAML scalars, so marshalling is both the quoting
		// rule and the injection guard: a value carrying a newline or a colon
		// comes back quoted rather than becoming two keys.
		fmt.Fprintf(&add, "%s: %s\n", k, v)
	}
	return []byte(prefix + rest[:end+1] + add.String() + rest[end+1:]), nil
}

// zipEpoch is the modification time every entry carries: the oldest timestamp
// the zip format can represent. A fixed one is the point — a real one would move
// the bytes on every build without the content moving.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// writeZip serialises the files canonically (ADR-027 decision 2): entries sorted
// by path, no directory entries, every modification time at the zip epoch,
// external attributes zero, fixed compression level.
//
// Setting Modified makes the writer add an extended-timestamp extra field, which
// ADR-027 asked to keep out. It is kept because the alternative is the
// deprecated ModifiedDate/ModifiedTime fields, and the reason the ADR wanted no
// extra field — that it moves with the clock — does not apply to a field whose
// content is entirely determined by a constant.
//
// The result is that the same packager version over the same content produces
// the same bytes, which is what makes content_hash mean "is this the file I
// had" rather than "was this built by the same process invocation". Across
// packager versions it is deliberately not promised: keeping it would pin a
// compression library's implementation as a public contract.
func writeZip(files []exportFile, prefix string) ([]byte, error) {
	sorted := make([]exportFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, deflateLevel)
	})
	for _, f := range sorted {
		h := &zip.FileHeader{Name: prefix + f.path, Method: zip.Deflate, Modified: zipEpoch}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// manifestHash is the canonical content hash of ADR-027 decision 1: SHA-256 over
// the canonical JSON of {package-relative path: sha256(bytes)}.
//
// Two exclusions, both load-bearing. Zip metadata — entry order, mtime, external
// attributes, compression method — because it moves without the content moving,
// which is the whole reason a second hash exists. And the manifest itself,
// because the manifest carries this value and carries `packaged_at`: including
// it would make two byte-identical builds hash differently, which is the exact
// question this answers independently of.
//
// encoding/json is the canonicalisation: it sorts map keys and writes no
// insignificant whitespace, which is the serialisation $defs/manifestHashInput
// describes.
func manifestHash(files []exportFile) (string, error) {
	digests := make(map[string]string, len(files))
	for _, f := range files {
		if f.path == ManifestFile {
			continue
		}
		sum := sha256.Sum256(f.data)
		digests[f.path] = hex.EncodeToString(sum[:])
	}
	canonical, err := json.Marshal(digests)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// exportFS presents the files as the tree they will be inside the package root,
// so the static validation can be run over them before the zip exists. skillpkg
// takes an fs.FS and nothing else, which is what makes this possible without a
// second reader (packaging-design §2.1).
func exportFS(files []exportFile) fs.FS {
	m := make(fstest.MapFS, len(files))
	for _, f := range files {
		m[f.path] = &fstest.MapFile{Data: f.data}
	}
	return m
}

// marshalManifest writes skillhub-manifest.json. Indented and newline
// terminated: it is a file a user opens, and the hash that identifies the
// package's content is computed over the other files, never over this one, so
// the formatting costs nothing.
func marshalManifest(m Manifest) ([]byte, error) {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
