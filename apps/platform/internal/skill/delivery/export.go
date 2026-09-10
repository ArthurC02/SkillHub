package packaging

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

const (
	ManifestFile = "skillhub-manifest.json"
	InstallFile  = "INSTALL.md"
	testCasesDir = "test-cases/"
)

const deflateLevel = 5

var excludedDirs = map[string]bool{
	".git": true, ".github": true, "node_modules": true,
	"__pycache__": true, ".venv": true, ".tox": true, ".mypy_cache": true,
	".aws": true, ".azure": true, ".docker": true, ".kube": true, ".ssh": true,
}

var excludedCredentialFiles = map[string]bool{
	".git-credentials": true, ".netrc": true, ".npmrc": true, ".pypirc": true,
	"application_default_credentials.json": true,
}

type exportFile struct {
	path string
	data []byte
}

func collect(fsys fs.FS) ([]exportFile, []ExcludedFile, error) {
	var out []exportFile
	var dropped []ExcludedFile
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && excludedDirs[lastSegment(path)] {

				dropped = append(dropped, ExcludedFile{Path: path + "/", Reason: ReasonExcludedDir})
				return fs.SkipDir
			}
			return nil
		}
		if ok, reason := travels(path, d); !ok {
			dropped = append(dropped, ExcludedFile{Path: path, Reason: reason})
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
		return nil, nil, err
	}
	return out, dropped, nil
}

func travels(path string, d fs.DirEntry) (bool, string) {
	if !fs.ValidPath(path) || path == "." || strings.ContainsAny(path, `\`) {
		return false, ReasonUnsafePath
	}
	if strings.HasPrefix(path, "/") || hasDriveLetter(path) {
		return false, ReasonUnsafePath
	}
	lowerPath := strings.ToLower(path)
	for _, seg := range strings.Split(lowerPath, "/") {
		if seg == ".." || seg == "." {
			return false, ReasonUnsafePath
		}
		if excludedDirs[seg] {
			return false, ReasonExcludedDir
		}
	}
	name := lastSegment(lowerPath)
	envTemplate := strings.HasSuffix(name, ".example") || strings.HasSuffix(name, ".sample") || strings.HasSuffix(name, ".template")
	if name == ".env" || (strings.HasPrefix(name, ".env.") && !envTemplate) || excludedCredentialFiles[name] ||
		hasPathSuffix(lowerPath, ".config/gcloud/credentials.db") ||
		hasPathSuffix(lowerPath, ".config/gcloud/access_tokens.db") ||
		hasPathSuffix(lowerPath, ".config/gh/hosts.yml") {
		return false, ReasonCredentialFile
	}

	if info, err := d.Info(); err != nil || !info.Mode().IsRegular() {
		return false, ReasonNotRegularFile
	}
	return true, ""
}

func pathUsesExcludedDir(path string) bool {
	for _, seg := range strings.Split(strings.ToLower(path), "/") {
		if excludedDirs[seg] {
			return true
		}
	}
	return false
}

func hasPathSuffix(path, suffix string) bool {
	return path == suffix || strings.HasSuffix(path, "/"+suffix)
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
		// A JSON scalar is also a valid YAML scalar, so this both formats the
		// value and quotes anything containing a colon or newline.
		fmt.Fprintf(&add, "%s: %s\n", k, v)
	}
	return []byte(prefix + rest[:end+1] + add.String() + rest[end+1:]), nil
}

var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func writeZip(files []exportFile, prefix string) ([]byte, error) {
	sorted := make([]exportFile, len(files))
	copy(sorted, files)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, deflateLevel)
	})
	for i, f := range sorted {

		if i > 0 && sorted[i-1].path == f.path {
			return nil, fmt.Errorf("two files claim the path %q", f.path)
		}

		// Modified would also write an extended-timestamp extra field, changing
		// the archive bytes; ModifiedDate holds a fixed value without that.
		//nolint:staticcheck // SA1019: Modified would change the bytes; see above.
		h := &zip.FileHeader{Name: prefix + f.path, Method: zip.Deflate, ModifiedDate: 33}
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

func exportFS(files []exportFile) fs.FS {
	m := make(fstest.MapFS, len(files))
	for _, f := range files {
		m[f.path] = &fstest.MapFile{Data: f.data}
	}
	return m
}

func marshalManifest(m Manifest) ([]byte, error) {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
