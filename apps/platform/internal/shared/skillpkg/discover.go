package skillpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	PluginSchemaID     = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	PluginSpecRevision = "agent-plugins.org, Agent Plugins 1.0.0"
)

const (
	CodePluginComponent    = "plugin-component"
	CodePluginManifest     = "plugin-manifest"
	CodeForeignManifest    = "foreign-plugin-manifest"
	CodeNoSkillFound       = "no-skill-found"
	CodePluginUnknownField = "plugin-manifest-unknown-field"
	CodeDuplicateSkillName = "duplicate-skill-name"
)

type SourceShape string

const (
	ShapeSkill  SourceShape = "skill"
	ShapePlugin SourceShape = "plugin"
	ShapeTree   SourceShape = "tree"
)

const (
	pluginManifestName = "plugin.json"
	pluginSkillsDir    = "skills"
	pluginMCPName      = "mcp.json"
)

var pluginNameRule = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// A namespace segment is the specification's own escape hatch for client
// behaviour, so matching its reverse-domain shape is what keeps this file free
// of any particular client's name.
var reverseDomainDir = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9][a-z0-9-]*)+$`)

var pluginManifestFields = []string{
	"$schema", "name", "version", "description", "author",
	"homepage", "repository", "license", "keywords", "extensions",
}

type PluginFacts struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Repository string `json:"repository,omitempty"`
}

type Discovery struct {
	Shape    SourceShape  `json:"shape"`
	Plugin   *PluginFacts `json:"plugin,omitempty"`
	Skills   []string     `json:"skills"`
	Excluded []Finding    `json:"excluded,omitempty"`
	Findings []Finding    `json:"findings"`
	Blocked  bool         `json:"blocked"`
}

func (d *Discovery) add(sev Severity, code, p, msg string) {
	d.Findings = append(d.Findings, Finding{Severity: sev, Code: code, Path: p, Message: msg})
	if sev == SeverityError {
		d.Blocked = true
	}
}

func Discover(fsys fs.FS) Discovery {
	d := Discovery{Shape: ShapeTree}
	if facts, ok := d.readPluginManifest(fsys); ok {
		d.Shape, d.Plugin = ShapePlugin, facts
		d.collectPluginSkills(fsys)
		d.collectPluginComponents(fsys)
	} else if d.Blocked {
		return d
	} else if hasSkillFile(fsys, ".") {
		d.Shape, d.Skills = ShapeSkill, []string{"."}
	} else {
		d.collectTreeSkills(fsys)
	}
	if len(d.Skills) == 0 && !d.Blocked {
		d.add(SeverityError, CodeNoSkillFound, "", d.notFoundMessage())
	}
	return d
}

func (d *Discovery) notFoundMessage() string {
	if d.Shape == ShapePlugin {
		return "這個來源的 plugin.json 合規，但 skills/ 之下沒有任何一個直接子目錄含有 SKILL.md。" +
			"規格把 Skill 的位置固定在那裡，而且不往更深層找。"
	}
	return "這個來源裡找不到任何 SKILL.md。平台找過三個地方：來源根目錄的 SKILL.md（單一 Skill）、" +
		"來源根目錄的 plugin.json 所指的 skills/（Agent Plugin），以及整棵目錄樹裡每一個含 SKILL.md 的資料夾。" +
		"三個都沒有，代表這個來源裡沒有合規格的 Agent Skill。"
}

func (d *Discovery) readPluginManifest(fsys fs.FS) (*PluginFacts, bool) {
	raw, err := fs.ReadFile(fsys, pluginManifestName)
	if err != nil {
		return nil, false
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		d.add(SeverityInfo, CodeForeignManifest, pluginManifestName,
			"根目錄的 plugin.json 不是有效的 JSON，平台不把它當作 plugin manifest 讀；這個來源改以整棵目錄樹尋找 SKILL.md。")
		return nil, false
	}
	schema, _ := manifest["$schema"].(string)
	if schema != PluginSchemaID {
		d.add(SeverityInfo, CodeForeignManifest, pluginManifestName, foreignManifestMessage(schema))
		return nil, false
	}

	name, _ := manifest["name"].(string)
	if !validPluginName(name) {
		d.add(SeverityError, CodePluginManifest, pluginManifestName,
			"這份 plugin.json 宣告了 Agent Plugin 規格，但 name 不合規格：必須是 1–64 個字元、"+
				"只含小寫英數與 - 和 .、首尾為英數，且不含連續的 -- 或 .. 。")
		return nil, false
	}
	if unknown := unknownManifestFields(manifest); len(unknown) > 0 {
		d.Findings = append(d.Findings, Finding{
			Severity: SeverityInfo, Code: CodePluginUnknownField, Path: pluginManifestName,
			Message: "plugin.json 有規格未定義的欄位，平台記下它們並繼續匯入。", Details: unknown})
	}
	facts := &PluginFacts{Name: name}
	facts.Version, _ = manifest["version"].(string)
	facts.Repository, _ = manifest["repository"].(string)
	return facts, true
}

func foreignManifestMessage(schema string) string {
	if schema == "" {
		return "根目錄的 plugin.json 沒有 $schema，平台無法確認它遵循哪一份規格，因此不把它當作 plugin manifest 讀；" +
			"這個來源改以整棵目錄樹尋找 SKILL.md，裡面的 Skill 照樣匯得進來。"
	}
	return "根目錄的 plugin.json 宣告的是另一份規格（" + schema + "），平台只讀公開跨供應商的那一份，" +
		"因此不把它當作 plugin manifest 讀；這個來源改以整棵目錄樹尋找 SKILL.md，裡面的 Skill 照樣匯得進來。"
}

func validPluginName(name string) bool {
	return len(name) >= 1 && len(name) <= maxNameLen &&
		!strings.Contains(name, "--") && !strings.Contains(name, "..") &&
		pluginNameRule.MatchString(name)
}

func unknownManifestFields(manifest map[string]any) []string {
	known := make(map[string]bool, len(pluginManifestFields))
	for _, f := range pluginManifestFields {
		known[f] = true
	}
	var unknown []string
	for field := range manifest {
		if !known[field] {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	return unknown
}

func (d *Discovery) collectPluginSkills(fsys fs.FS) {
	entries, err := fs.ReadDir(fsys, pluginSkillsDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := path.Join(pluginSkillsDir, entry.Name())
		if hasSkillFile(fsys, dir) {
			d.Skills = append(d.Skills, dir)
		}
	}
}

func (d *Discovery) collectPluginComponents(fsys fs.FS) {
	if info, err := fs.Stat(fsys, pluginMCPName); err == nil && !info.IsDir() {
		d.Excluded = append(d.Excluded, Finding{
			Severity: SeverityInfo, Code: CodePluginComponent, Path: pluginMCPName,
			Message: "這個 Plugin 宣告了 MCP server。MCP server 的定義本身就是執行指令，平台只揭露它、不匯入也不執行。"})
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && reverseDomainDir.MatchString(entry.Name()) {
			d.Excluded = append(d.Excluded, Finding{
				Severity: SeverityInfo, Code: CodePluginComponent, Path: entry.Name() + "/",
				Message: "這是規格劃給宿主的命名空間目錄，內容由各家用戶端自行定義。平台只揭露它、不匯入也不執行。"})
		}
	}
}

func (d *Discovery) collectTreeSkills(fsys fs.FS) {
	_ = fs.WalkDir(fsys, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil //nolint:nilerr // an unreadable directory holds no skill we can package
		}
		if hasSkillFile(fsys, p) {
			d.Skills = append(d.Skills, p)
		}
		return nil
	})
	sort.Strings(d.Skills)
}

func hasSkillFile(fsys fs.FS, dir string) bool {
	info, err := fs.Stat(fsys, path.Join(dir, "SKILL.md"))
	return err == nil && info.Mode().IsRegular()
}

func SubtreeHash(fsys fs.FS) (string, error) {
	sum := sha256.New()
	err := fs.WalkDir(fsys, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(sum, "%s\x00%d\x00", p, len(data))
		sum.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
