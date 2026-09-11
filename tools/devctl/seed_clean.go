package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	gen009SkillsRelDir  = "docs/plans/mvp/m5/gen009-round-d/skills"
	gen009ExpectedCount = 20

	goldensetCorpusRelDir  = "tools/goldenset/corpus"
	goldensetExpectedCount = 31

	seedDevLoginUser = "seed-importer"
)

var seedExclusions = map[string]string{
	"tools/goldenset/corpus/documents/minimax-docx.md": "frontmatter declares `triggers`, which the Agent Skills spec does not define; the platform's spec validator rejects the package with 422 (04 丙-84 ②). Left unedited on purpose: it is goldenset retrieval corpus, and changing it would move that batch's measurements.",
}

func seedExpectedUploads() int {
	return gen009ExpectedCount + goldensetExpectedCount - len(seedExclusions)
}

func partitionSeedEntries(all []seedEntry) (upload, excluded []seedEntry, err error) {
	seen := map[string]bool{}
	for _, e := range all {
		if _, ok := seedExclusions[e.provenance]; ok {
			seen[e.provenance] = true
			excluded = append(excluded, e)
			continue
		}
		upload = append(upload, e)
	}
	for path := range seedExclusions {
		if !seen[path] {
			return nil, nil, fmt.Errorf(
				"seed-clean: exclusion %s matched no collected file — the path moved or the batch changed; "+
					"fix or remove the entry rather than leaving one that excludes nothing", path)
		}
	}
	if len(upload) != seedExpectedUploads() {
		return nil, nil, fmt.Errorf("seed-clean: %d entries to upload, want %d", len(upload), seedExpectedUploads())
	}
	return upload, excluded, nil
}

func writeSeedExclusions(out io.Writer, excluded []seedEntry) {
	for _, e := range excluded {
		fmt.Fprintf(out, "excluded: %s\n          %s\n", e.provenance, seedExclusions[e.provenance])
	}
}

type seedEntry struct {
	name       string
	provenance string
	skillMD    []byte
}

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

func retryAfter(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {

		return time.Duration(seconds)*time.Second + time.Second
	}
	return time.Duration(attempt) * time.Second
}

func seedCatalogSearch(client *http.Client, api, q string) (int, error) {
	const maxAttempts = 4
	target := api + "/api/skills/search?limit=1&q=" + url.QueryEscape(q)
	for attempt := 1; ; attempt++ {
		resp, err := client.Get(target)
		if err != nil {
			return 0, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4000))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			time.Sleep(retryAfter(resp.Header.Get("Retry-After"), attempt))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("GET /api/skills/search answered %d: %s", resp.StatusCode, firstLine(string(b)))
		}
		var body struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal(b, &body); err != nil {
			return 0, fmt.Errorf("GET /api/skills/search: %w", err)
		}
		return body.Total, nil
	}
}

func verifyEnrichmentReached(client *http.Client, api, name, skillID string, out io.Writer) error {
	if skillID == "" {
		return nil
	}
	resp, err := client.Get(api + "/api/skills/" + url.PathEscape(skillID))
	if err != nil {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 20000))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var body struct {
		Enrichment struct {
			Status string `json:"status"`
		} `json:"enrichment"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil
	}
	if body.Enrichment.Status != "pending" {
		fmt.Fprintf(out, "index check: the first package came back enriched (status %q) — intent search will work\n",
			body.Enrichment.Status)
		return nil
	}
	return fmt.Errorf(
		"seed-clean: the first package imported but was left unindexed (enrichment.status is \"pending\" for %q).\n"+
			"  A pending package is kept out of every catalog query, keyword search and browse included, so the demo's\n"+
			"  catalog would stay empty however many packages followed; nothing after this one was uploaded.\n"+
			"  Start apps/llm and the model gateway it calls, check that the API's LLM_SERVICE_URL points at it, then seed again.\n"+
			"  What is already uploaded is not lost while clean mode keeps running: the worker's hourly enrichment backfill\n"+
			"  indexes pending packages once apps/llm answers. A restart does lose it, because the PGlite carrier is in memory.",
		name)
}

const seedVerifyProbes = 5

func verifyCatalogVisible(client *http.Client, api string, uploaded []seedEntry, out io.Writer) error {
	probes := uploaded
	if len(probes) > seedVerifyProbes {
		probes = probes[:seedVerifyProbes]
	}
	tried := make([]string, 0, len(probes))
	for _, e := range probes {
		total, err := seedCatalogSearch(client, api, e.name)
		if err != nil {
			return fmt.Errorf("seed-clean: catalog visibility check: %w", err)
		}
		tried = append(tried, fmt.Sprintf("%q→total=%d", e.name, total))
		if total > 0 {
			fmt.Fprintf(out, "catalog check: GET /api/skills/search?q=%s returns total=%d — the seeded skills are visible on the demo's own screen\n", e.name, total)
			return nil
		}
	}
	return fmt.Errorf(
		"seed-clean: %d package(s) uploaded, but the public catalog search finds none of them (%s).\n"+
			"  GET /api/skills/search returns only workspaces with `is_catalog = true`, and no HTTP endpoint sets that flag.\n"+
			"  tools/cleanmode/start.mjs grants it to %q before the API starts; a deployment seeded some other way needs\n"+
			"    UPDATE workspaces SET is_catalog = true WHERE name = '%s';\n"+
			"  and then `devctl seed-clean` again. Leaving the data in place unseen is what 04 丙-84 ① recorded",
		len(uploaded), strings.Join(tried, ", "), seedDevLoginUser, seedDevLoginUser)
}

func seedClean(root string, args []string, out io.Writer) error {
	dryRun := false
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		default:
			return fmt.Errorf("seed-clean: unknown argument %q (only --dry-run is accepted)", a)
		}
	}

	all, err := collectSeedEntries(root)
	if err != nil {
		return err
	}
	entries, excluded, err := partitionSeedEntries(all)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Fprintf(out, "seed-clean --dry-run: %d skill(s) would be uploaded to %s; no request sent\n", len(entries), seedCleanAPIBase())
		for i, e := range entries {
			fmt.Fprintf(out, "  [%3d] %-55s source=%s\n", i+1, e.name, e.provenance)
		}
		writeSeedExclusions(out, excluded)
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
			continue
		}
		if imported == 1 {
			var created struct {
				SkillID string `json:"skill_id"`
			}
			_ = json.Unmarshal([]byte(body), &created)
			if err := verifyEnrichmentReached(client, api, e.name, created.SkillID, out); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(out, "\nimported=%d failed=%d excluded=%d\n", imported, failed, len(excluded))
	writeSeedExclusions(out, excluded)
	if failed > 0 {
		return fmt.Errorf("seed-clean: %d of %d upload(s) failed", failed, len(entries))
	}
	return verifyCatalogVisible(client, api, entries, out)
}
