package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeContextMapFixture 鋪出這道檢查讀的三份清單：ADR-032 §1 的表格、
// depguard 規則，以及 apps/platform/internal/ 底下的套件目錄。
func writeContextMapFixture(t *testing.T, adr, lint string, packages []string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/adr/"+contextMapADR, adr)
	write("apps/platform/.golangci.yml", lint)
	for _, name := range packages {
		write("apps/platform/internal/"+name+"/doc.go", "package "+filepath.Base(name)+"\n")
	}
	return root
}

// 真實 §1 表格的形狀：一列多個套件、儲存格夾全形括號的註解（而註解裡**也有**
// 反引號，`SaveVersion` 就是那個陷阱）、以及 `platform/*`／`api/gen` 這種
// 不是單一目錄名的寫法。表格後面刻意再放一個 `### ` 標題與另一張表，
// 因為 ADR-032 有四張表，抓錯一張這道檢查就在比對別的東西。
const contextMapADRFixture = "### 1. Context 對照表\n" +
	"\n" +
	"| Bounded Context | 類型 | internal/ 套件 | 需求 ID 前綴 |\n" +
	"| --- | --- | --- | --- |\n" +
	"| Skill Registry | Core | `registry`、`skillpkg` | SKILL |\n" +
	"| Trust | Core | `ingest`（匯入管線；`SaveVersion` 是唯一驗證路徑） | SKILL |\n" +
	"| —（跨切面，非 context） | Generic | `audit`、`platform/*`、`apiserver`、`api/gen` | — |\n" +
	"\n" +
	"### 2. 關係\n" +
	"\n" +
	"| 關係 | 機制 | 適用 |\n" +
	"| --- | --- | --- |\n" +
	"| 同步查詢 | import `registryz` 的公開 API | 事實 |\n"

// depguard 的規則只以 files: 的 glob 認人，所以 fixture 也只留那一半。
const contextMapLintFixture = `      depguard:
        rules:
          registry:
            files:
              - "**/internal/registry/**"
              - "!$test"
          generic:
            files:
              - "**/internal/audit/**"
              - "**/internal/skillpkg/**"
              - "**/internal/platform/**"
              - "!$test"
          ingest:
            files:
              - "**/internal/ingest/**"
              - "!$test"
`

func TestContextMapProblems(t *testing.T) {
	t.Parallel()

	// 對得上的基準：五個套件目錄對上 §1 的七個 token（`platform/*` → platform、
	// `api/gen` → api），非 Generic 的三個都有 depguard 規則。
	packages := []string{"registry", "skillpkg", "ingest", "audit", "platform", "apiserver", "api"}

	tests := []struct {
		name     string
		adr      string
		lint     string
		packages []string
		want     string // 預期唯一問題的子字串；空字串代表要全綠
	}{
		{
			name:     "three lists agree",
			adr:      contextMapADRFixture,
			lint:     contextMapLintFixture,
			packages: packages,
		},
		{
			name:     "package missing from the ADR table",
			adr:      contextMapADRFixture,
			lint:     contextMapLintFixture,
			packages: append(append([]string{}, packages...), "billing"),
			want:     "apps/platform/internal/billing is not listed in",
		},
		{
			name:     "ADR lists a package that does not exist",
			adr:      strings.Replace(contextMapADRFixture, "`registry`、`skillpkg`", "`registry`、`skillpkg`、`ghost`", 1),
			lint:     contextMapLintFixture,
			packages: packages,
			want:     `§1 lists "ghost" but apps/platform/internal/ghost does not exist`,
		},
		{
			name:     "bounded context with no depguard rule",
			adr:      contextMapADRFixture,
			lint:     strings.Replace(contextMapLintFixture, "              - \"**/internal/ingest/**\"\n", "", 1),
			packages: packages,
			want:     `§1 puts "ingest" in a bounded context but apps/platform/.golangci.yml has no depguard rule`,
		},
		{
			name:     "generic packages need no depguard rule",
			adr:      contextMapADRFixture,
			lint:     strings.Replace(contextMapLintFixture, "              - \"**/internal/audit/**\"\n", "", 1),
			packages: packages,
		},
		{
			name:     "depguard rule for a package that is gone",
			adr:      contextMapADRFixture,
			lint:     contextMapLintFixture,
			packages: []string{"skillpkg", "ingest", "audit", "platform", "apiserver", "api"},
			want:     "guards apps/platform/internal/registry but that package does not exist",
		},
		{
			name: "a package name the parser cannot read is reported, not skipped",
			adr: strings.Replace(contextMapADRFixture,
				"`registry`、`skillpkg`", "`registry`、`internal/db/gen/*`", 1),
			lint:     contextMapLintFixture,
			packages: packages,
			// skillpkg 消失也會被報，所以這個案例預期兩個問題；只斷言解析那一條。
			want: `lists "internal/db/gen/*", which is not a package directory this check can read`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := writeContextMapFixture(t, test.adr, test.lint, test.packages)
			problems := contextMapProblems(root)
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %#v", problems)
				}
				return
			}
			for _, problem := range problems {
				if strings.Contains(problem, test.want) {
					return
				}
			}
			t.Fatalf("no problem mentions %q, got %#v", test.want, problems)
		})
	}
}
