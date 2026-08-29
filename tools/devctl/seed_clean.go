package main

// seed_clean.go implements `devctl seed-clean`, the PORT-007 demo seeder for
// the clean test mode: every skill it uploads must trace back to a real,
// already-committed file — no manifest, no invented bytes.
//
// A 2026-08-29 inventory (docs/plans/03-work-items.md §20, PORT-007) found
// only two source batches that are both offline-usable and byte-traceable:
//
//   - docs/plans/mvp/m5/gen009-round-d/skills/*.md — 20 generated SKILL.md
//   - tools/goldenset/corpus/**/*.md               — 31 curated SKILL.md
//
// Two other candidates were ruled out and must not be added back here: a
// report whose per-run data only ever lived in a developer's local database,
// and a curated list whose bytes are deliberately not committed (4 of them
// are not redistributable, and PORT-005 requires offline dependencies).
//
// This command sends bytes only, through the same public HTTP API
// tools/content/import_seed.py uses (dev login, then one package upload per
// skill) — so the clean mode never grows a second data path (PORT-008).
//
// Deliberately out of scope: corpus.json / results.json / human-verdicts.tsv
// in the gen009 batch carry Run status and eval verdicts, but there is no API
// that lets a seeder set Run state directly (ADR-008: the Postgres state
// machine is the only source of truth for Run status), and a trace event must
// come from an actual Run, never be planted (PORT-007). Skill bytes are the
// only thing this command manufactures a request for.
import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Counts the 2026-08-29 inventory fixed (docs/plans/03-work-items.md §20). A
// mismatch means a source file went missing or the batch grew without this
// command being told about it — either way, a loud failure beats a quietly
// shorter demo (02:PORT-007: "指不回去的即不得使用").
const (
	gen009SkillsRelDir  = "docs/plans/mvp/m5/gen009-round-d/skills"
	gen009ExpectedCount = 20

	goldensetCorpusRelDir  = "tools/goldenset/corpus"
	goldensetExpectedCount = 31

	seedDevLoginUser = "seed-importer" // same default as tools/content/import_seed.py
)

type seedEntry struct {
	name       string // skill name, for logging only
	provenance string // path relative to the repo root; must os.Stat-resolve
	skillMD    []byte
}

// collectSeedEntries walks both PORT-007 source batches under root and
// returns one entry per SKILL.md found. It fails loudly — never skips — when
// a batch's file count does not match what the 2026-08-29 inventory recorded,
// or when a file does not look like a SKILL.md.
func collectSeedEntries(root string) ([]seedEntry, error) {
	gen009, err := collectMarkdownSkills(root, gen009SkillsRelDir, gen009ExpectedCount)
	if err != nil {
		return nil, err
	}
	goldenset, err := collectMarkdownSkills(root, goldensetCorpusRelDir, goldensetExpectedCount)
	if err != nil {
		return nil, err
	}
	return append(gen009, goldenset...), nil
}

func collectMarkdownSkills(root, relDir string, want int) ([]seedEntry, error) {
	dir := filepath.Join(root, filepath.FromSlash(relDir))
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("seed-clean: read %s: %w", relDir, err)
	}
	sort.Strings(paths)
	if len(paths) != want {
		return nil, fmt.Errorf(
			"seed-clean: %s has %d SKILL.md file(s), want %d — a source file went missing or the batch grew; "+
				"investigate before changing the expected count (02:PORT-007 forbids silently using fewer than recorded)",
			relDir, len(paths), want,
		)
	}

	entries := make([]seedEntry, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("seed-clean: read %s: %w", path, err)
		}
		if !looksLikeSkillMD(data) {
			return nil, fmt.Errorf("seed-clean: %s does not start with YAML frontmatter (---); not a SKILL.md", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		// The provenance string is what PORT-007 requires every demo entry to
		// point back to; confirm it actually resolves rather than trusting the
		// string shape.
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return nil, fmt.Errorf("seed-clean: provenance %s does not resolve: %w", rel, err)
		}
		entries = append(entries, seedEntry{
			name:       strings.TrimSuffix(filepath.Base(path), ".md"),
			provenance: filepath.ToSlash(rel),
			skillMD:    data,
		})
	}
	return entries, nil
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func looksLikeSkillMD(data []byte) bool {
	data = bytes.TrimPrefix(data, utf8BOM)
	data = bytes.TrimLeft(data, " \t\r\n")
	return bytes.HasPrefix(data, []byte("---"))
}

// packSkillZip re-roots one SKILL.md's bytes into a package zip, the same
// shape import_seed.py's repack_skill produces: SKILL.md at the archive root.
func packSkillZip(md []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, err := w.Create("SKILL.md")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(md); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func seedCleanAPIBase() string {
	if v := strings.TrimSpace(os.Getenv("SKILLHUB_API")); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func seedCleanDevLogin(client *http.Client, api string) error {
	body, err := json.Marshal(map[string]string{"user": seedDevLoginUser})
	if err != nil {
		return err
	}
	resp, err := client.Post(api+"/auth/dev/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("seed-clean: dev login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("seed-clean: dev login failed (%d): %s — is DEV_LOGIN=1 set on the target deployment?", resp.StatusCode, firstLine(string(b)))
	}
	return nil
}

// seedCleanUpload waits out the import rate limiter rather than counting its
// refusals as failures. The limiter is not an obstacle to route around: it is
// NFR-001's brake on the two import endpoints, and this command sends fifty-one
// uploads back to back, which is exactly the shape it exists to slow down.
//
// This was found by running the seeder against a live deployment for the first
// time: 30 of 51 landed and 21 came back 429. No unit test would have shown it,
// because the httptest server the tests use has no limiter in front of it.
func seedCleanUpload(client *http.Client, api string, zipBytes []byte) (status int, body string, err error) {
	const maxAttempts = 6
	for attempt := 1; ; attempt++ {
		resp, err := client.Post(api+"/skills/import/upload", "application/zip", bytes.NewReader(zipBytes))
		if err != nil {
			return 0, "", err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || attempt == maxAttempts {
			return resp.StatusCode, string(b), nil
		}
		time.Sleep(retryAfter(resp.Header.Get("Retry-After"), attempt))
	}
}

// retryAfter honours the server's own number when it sends one; the fallback
// grows so a deployment whose limiter says nothing still gets backed off rather
// than hammered.
func retryAfter(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
		// One second past what was asked for: the limiter's window and this
		// client's clock are not the same clock.
		return time.Duration(seconds)*time.Second + time.Second
	}
	return time.Duration(attempt) * time.Second
}

// seedClean is the entry point for `devctl seed-clean [--dry-run]`.
func seedClean(root string, args []string, out io.Writer) error {
	dryRun := false
	for _, a := range args {
		if a != "--dry-run" {
			return fmt.Errorf("seed-clean: unknown argument %q (only --dry-run is accepted)", a)
		}
		dryRun = true
	}

	entries, err := collectSeedEntries(root)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Fprintf(out, "seed-clean --dry-run: %d skill(s) would be uploaded to %s; no request sent\n", len(entries), seedCleanAPIBase())
		for i, e := range entries {
			fmt.Fprintf(out, "  [%3d] %-55s source=%s\n", i+1, e.name, e.provenance)
		}
		return nil
	}

	api := seedCleanAPIBase()
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client := &http.Client{Jar: jar}
	if err := seedCleanDevLogin(client, api); err != nil {
		return err
	}

	imported, failed := 0, 0
	for i, e := range entries {
		zipBytes, err := packSkillZip(e.skillMD)
		if err != nil {
			return fmt.Errorf("seed-clean: pack %s: %w", e.provenance, err)
		}
		status, body, err := seedCleanUpload(client, api, zipBytes)
		if err != nil {
			return fmt.Errorf("seed-clean: upload %s: %w", e.provenance, err)
		}
		ok := status == http.StatusCreated
		if ok {
			imported++
		} else {
			failed++
		}
		fmt.Fprintf(out, "[%3d/%d] %-55s source=%-60s -> %d\n", i+1, len(entries), e.name, e.provenance, status)
		if !ok {
			fmt.Fprintf(out, "          %s\n", firstLine(body))
		}
	}
	fmt.Fprintf(out, "\nimported=%d failed=%d\n", imported, failed)
	if failed > 0 {
		return fmt.Errorf("seed-clean: %d of %d upload(s) failed", failed, len(entries))
	}
	return nil
}
