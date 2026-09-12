package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type licenseTarget struct {
	ecosystem string
	dir       string
}

var licenseTargets = []licenseTarget{
	{ecosystem: "npm", dir: "apps/web"},
	{ecosystem: "go", dir: "apps/platform"},
	{ecosystem: "go", dir: "apps/sandbox"},
	{ecosystem: "python", dir: "apps/llm"},
	{ecosystem: "npm", dir: "infra/images/runtime-agent-sdk"},
}

var allowedLicenses = map[string]bool{
	"0bsd": true, "apache-2.0": true, "blueoak-1.0.0": true, "bsd-2-clause": true, "bsd-3-clause": true,
	"cc0-1.0": true, "isc": true, "mit": true, "mit-0": true, "mpl-2.0": true, "psf-2.0": true,
	"python-2.0": true, "unlicense": true, "zlib": true,
}

var acceptedLicenses = []struct {
	ecosystem string
	name      string
	license   string
	reason    string
}{
	{"npm", "@anthropic-ai/claude-agent-sdk", "SEE LICENSE IN", "Anthropic's commercial terms, pinned and revalidated with the runtime image"},
	{"go", "github.com/segmentio/asm", "Unknown", "its LICENSE is MIT No Attribution (MIT-0), which go-licenses does not classify"},
}

var classifierLicenses = map[string]string{
	"License :: OSI Approved :: MIT License":                          "MIT",
	"License :: OSI Approved :: MIT No Attribution License (MIT-0)":   "MIT-0",
	"License :: OSI Approved :: BSD License":                          "BSD-3-Clause OR BSD-2-Clause",
	"License :: OSI Approved :: Apache Software License":              "Apache-2.0",
	"License :: OSI Approved :: ISC License (ISCL)":                   "ISC",
	"License :: OSI Approved :: Mozilla Public License 2.0 (MPL 2.0)": "MPL-2.0",
	"License :: OSI Approved :: Python Software Foundation License":   "PSF-2.0",
	"License :: OSI Approved :: The Unlicense (Unlicense)":            "Unlicense",
	"License :: OSI Approved :: zlib/libpng License":                  "Zlib",
	"License :: CC0 1.0 Universal (CC0 1.0) Public Domain Dedication": "CC0-1.0",
}

type packageLicense struct {
	name    string
	version string
	license string
}

type pypiLicenseInfo struct {
	LicenseExpression string   `json:"license_expression"`
	License           string   `json:"license"`
	Classifiers       []string `json:"classifiers"`
}

type exportedPin struct {
	name    string
	version string
}

func licenseAudit(root string, toolchain map[string]string) (fail, note []string, err error) {
	for _, target := range licenseTargets {
		dir := filepath.Join(root, filepath.FromSlash(target.dir))
		var packages []packageLicense
		switch target.ecosystem {
		case "npm":
			packages, err = npmShippedLicenses(dir)
		case "go":
			packages, err = goLicenses(dir, toolchain["go_licenses"])
		case "python":
			packages, err = pythonLicenses(dir)
		default:
			err = fmt.Errorf("unknown ecosystem %q", target.ecosystem)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("license %s %s: %w", target.ecosystem, target.dir, err)
		}
		targetFail, targetNote := judgeLicenses(target, packages)
		fail = append(fail, targetFail...)
		note = append(note, targetNote...)
	}
	return fail, note, nil
}

func judgeLicenses(target licenseTarget, packages []packageLicense) (fail, note []string) {
	for _, pkg := range packages {
		if licenseAllowed(pkg.license) {
			continue
		}
		line := fmt.Sprintf("license %s %s: %s %q", target.ecosystem, target.dir, strings.TrimSpace(pkg.name+" "+pkg.version), pkg.license)
		if reason, ok := acceptedLicense(target.ecosystem, pkg); ok {
			note = append(note, line+" accepted: "+reason)
			continue
		}
		fail = append(fail, line+" is not on the allowlist")
	}
	return fail, note
}

func acceptedLicense(ecosystem string, pkg packageLicense) (string, bool) {
	for _, accepted := range acceptedLicenses {
		sameFamily := pkg.name == accepted.name ||
			strings.HasPrefix(pkg.name, accepted.name+"-") ||
			strings.HasPrefix(pkg.name, accepted.name+"/")
		if accepted.ecosystem == ecosystem && sameFamily && strings.HasPrefix(pkg.license, accepted.license) {
			return accepted.reason, true
		}
	}
	return "", false
}

func npmShippedLicenses(dir string) ([]packageLicense, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err != nil {
		return nil, err
	}
	return npmLockLicenses(data)
}

func npmLockLicenses(data []byte) ([]packageLicense, error) {
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
			License string `json:"license"`
			Dev     bool   `json:"dev"`
			Link    bool   `json:"link"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("package-lock.json: %w", err)
	}
	if len(lock.Packages) == 0 {
		return nil, errors.New("package-lock.json has no packages section")
	}
	var packages []packageLicense
	for key, entry := range lock.Packages {
		at := strings.LastIndex(key, "node_modules/")
		if at < 0 || entry.Dev || entry.Link {
			continue
		}
		packages = append(packages, packageLicense{name: key[at+len("node_modules/"):], version: entry.Version, license: entry.License})
	}
	sortPackages(packages)
	return packages, nil
}

func goLicenses(dir, version string) ([]packageLicense, error) {
	data, err := strictToolOutput(dir, "go", "run", "github.com/google/go-licenses/v2@v"+version,
		"report", "./...", "--ignore", "github.com/ArthurC02/skillhub")
	if err != nil {
		return nil, err
	}
	return goLicenseReport(data)
}

func goLicenseReport(data []byte) ([]packageLicense, error) {
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("go-licenses report: %w", err)
	}
	if len(records) == 0 {
		return nil, errors.New("go-licenses reported no packages, so nothing was checked")
	}
	var packages []packageLicense
	for _, record := range records {
		if len(record) != 3 {
			return nil, fmt.Errorf("go-licenses report line %q does not have three columns", strings.Join(record, ","))
		}
		packages = append(packages, packageLicense{name: record[0], license: record[2]})
	}
	sortPackages(packages)
	return packages, nil
}

func pythonLicenses(dir string) ([]packageLicense, error) {
	requirements, err := strictToolOutput(dir, "uv", "export", "--frozen", "--no-dev",
		"--no-emit-project", "--no-emit-local", "--no-hashes", "--quiet")
	if err != nil {
		return nil, err
	}
	pins, err := exportedPins(requirements)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	var packages []packageLicense
	for _, pin := range pins {
		info, err := pypiInfo(client, pin)
		if err != nil {
			return nil, err
		}
		packages = append(packages, packageLicense{name: pin.name, version: pin.version, license: pythonLicense(info)})
	}
	return packages, nil
}

func exportedPins(requirements []byte) ([]exportedPin, error) {
	var pins []exportedPin
	for _, line := range strings.Split(string(requirements), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		requirement, _, _ := strings.Cut(line, ";")
		name, version, pinned := strings.Cut(strings.TrimSpace(requirement), "==")
		if !pinned || strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return nil, fmt.Errorf("uv export line %q is not a name==version pin", line)
		}
		pins = append(pins, exportedPin{name: strings.TrimSpace(name), version: strings.TrimSpace(version)})
	}
	if len(pins) == 0 {
		return nil, errors.New("uv export listed no packages, so nothing was checked")
	}
	return pins, nil
}

func pypiInfo(client *http.Client, pin exportedPin) (pypiLicenseInfo, error) {
	address := "https://pypi.org/pypi/" + url.PathEscape(pin.name) + "/" + url.PathEscape(pin.version) + "/json"
	response, err := client.Get(address)
	if err != nil {
		return pypiLicenseInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return pypiLicenseInfo{}, fmt.Errorf("%s: %s", address, response.Status)
	}
	var body struct {
		Info pypiLicenseInfo `json:"info"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return pypiLicenseInfo{}, fmt.Errorf("%s: %w", address, err)
	}
	return body.Info, nil
}

func pythonLicense(info pypiLicenseInfo) string {
	if info.LicenseExpression != "" {
		return info.LicenseExpression
	}
	if _, valid := parseLicenseExpression(info.License); valid {
		return info.License
	}
	var mapped, unmapped []string
	for _, classifier := range info.Classifiers {
		if !strings.HasPrefix(classifier, "License ::") {
			continue
		}
		if id, ok := classifierLicenses[classifier]; ok {
			mapped = append(mapped, "("+id+")")
		} else {
			unmapped = append(unmapped, classifier)
		}
	}
	switch {
	case len(unmapped) > 0:
		return strings.Join(unmapped, "; ")
	case len(mapped) > 0:
		return strings.Join(mapped, " AND ")
	}
	return info.License
}

func licenseAllowed(expression string) bool {
	allowed, valid := parseLicenseExpression(expression)
	return allowed && valid
}

func parseLicenseExpression(expression string) (allowed, valid bool) {
	parser := spdxParser{tokens: strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(expression))}
	allowed, valid = parser.or()
	return allowed, valid && parser.pos == len(parser.tokens)
}

type spdxParser struct {
	tokens []string
	pos    int
}

func (p *spdxParser) next(operator string) bool {
	return p.pos < len(p.tokens) && strings.EqualFold(p.tokens[p.pos], operator)
}

func (p *spdxParser) or() (bool, bool) {
	allowed, valid := p.and()
	for valid && p.next("OR") {
		p.pos++
		right, rightValid := p.and()
		allowed, valid = allowed || right, rightValid
	}
	return allowed, valid
}

func (p *spdxParser) and() (bool, bool) {
	allowed, valid := p.term()
	for valid && p.next("AND") {
		p.pos++
		right, rightValid := p.term()
		allowed, valid = allowed && right, rightValid
	}
	return allowed, valid
}

func (p *spdxParser) term() (bool, bool) {
	if p.pos >= len(p.tokens) {
		return false, false
	}
	token := p.tokens[p.pos]
	p.pos++
	if token == "(" {
		allowed, valid := p.or()
		if !valid || !p.next(")") {
			return false, false
		}
		p.pos++
		return allowed, true
	}
	for _, operator := range []string{")", "AND", "OR", "WITH"} {
		if strings.EqualFold(token, operator) {
			return false, false
		}
	}
	if p.next("WITH") {
		p.pos += 2
		if p.pos > len(p.tokens) {
			return false, false
		}
	}
	return allowedLicenses[strings.ToLower(strings.TrimSuffix(token, "+"))], true
}

func strictToolOutput(dir, name string, args ...string) ([]byte, error) {
	data, err := auditToolOutput(dir, name, args...)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s %s printed nothing", name, strings.Join(args, " "))
	}
	return data, nil
}

func sortPackages(packages []packageLicense) {
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].name != packages[j].name {
			return packages[i].name < packages[j].name
		}
		return packages[i].version < packages[j].version
	})
}
