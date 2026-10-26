package creation

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const plainBody = "Read the input and report missing totals."

func draftWith(body string, files ...GeneratedFile) GeneratedSkill {
	return GeneratedSkill{Name: "invoice-check", Description: "Check invoices when asked.", Body: body, Files: files}
}

var inputSchema = GeneratedFile{Path: "scripts/input_schema.json", Content: `{"fields": []}`}

func TestTheShippedScriptsAreTheOnesThePythonSuiteTests(t *testing.T) {
	for embedded, source := range map[string]string{
		checkScriptSource: "../../../../llm/src/skillhub_llm/check_output.py",
		inputScriptSource: "../../../../llm/src/skillhub_llm/check_input.py",
	} {
		want, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if embedded != string(want) {
			t.Errorf("the embedded copy of %s differs from its source; copy it into shipped/ again", source)
		}
	}
}

func TestABodyNamingCheckOutputGetsThePlatformsScriptInItsFiles(t *testing.T) {
	got := assembleDraft(draftWith(plainBody+" Run scripts/check_output.py to check the limit."), "", nil)
	want := []GeneratedFile{{Path: "scripts/check_output.py", Content: checkScriptSource}}
	if !reflect.DeepEqual(got.Files, want) {
		t.Fatalf("files = %v, want only the platform's check script", got.Files)
	}
}

func TestTheModelsOwnContentAtTheCheckOutputPathIsReplaced(t *testing.T) {
	got := assembleDraft(draftWith(plainBody+" See scripts/check_output.py.", GeneratedFile{Path: "scripts/check_output.py", Content: "print('fake')"}), "", nil)
	if len(got.Files) != 1 || got.Files[0].Content != checkScriptSource {
		t.Fatalf("files = %v, want the model's script replaced by the platform's", got.Files)
	}
}

func TestADraftShippingAnInputSchemaRunsThePlatformsInputCheckFirst(t *testing.T) {
	got := assembleDraft(draftWith(plainBody, inputSchema, GeneratedFile{Path: "scripts/check_input.py", Content: "x"}), "", nil)
	want := []GeneratedFile{inputSchema, {Path: "scripts/check_input.py", Content: inputScriptSource}}
	if !reflect.DeepEqual(got.Files, want) {
		t.Fatalf("files = %v, want the schema and the platform's input check", got.Files)
	}
	if !strings.HasPrefix(got.Body, plainBody+"\n\n## Input check (first step)") || !strings.Contains(got.Body, "`python scripts/check_input.py input.json`") {
		t.Fatalf("body = %q, want the input check section after the model's body", got.Body)
	}
}

func TestABodyAlreadyRunningTheInputCheckGetsNoSecondSection(t *testing.T) {
	body := plainBody + " First run `python scripts/check_input.py input.json`."
	if got := assembleDraft(draftWith(body, inputSchema), "", nil); got.Body != body {
		t.Fatalf("body = %q, want it unchanged", got.Body)
	}
}

func TestADraftWithoutAnInputSchemaGetsNoInputCheck(t *testing.T) {
	got := assembleDraft(draftWith(plainBody), "", nil)
	if got.Body != plainBody || len(got.Files) != 0 {
		t.Fatalf("draft = %+v, want it unchanged", got)
	}
}

func TestAShippedScriptTheBodyNeverRunsGetsAScriptsSection(t *testing.T) {
	got := assembleDraft(draftWith(plainBody, GeneratedFile{Path: "scripts/fee.py", Content: "print(1)"}), "", nil)
	if want := plainBody + "\n\n" + scriptsSection + "\n- `python scripts/fee.py`\n"; got.Body != want {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func TestAShippedScriptTheBodyAlreadyRunsLeavesTheBodyAlone(t *testing.T) {
	body := "Run `python scripts/fee.py 100` and report the result."
	if got := assembleDraft(draftWith(body, GeneratedFile{Path: "scripts/fee.py", Content: "print(1)"}), "", nil); got.Body != body {
		t.Fatalf("body = %q, want it unchanged", got.Body)
	}
}

func TestAShippedReferenceTheBodyNeverLinksGetsAReferencesSection(t *testing.T) {
	got := assembleDraft(draftWith(plainBody, GeneratedFile{Path: "references/rates.md", Content: "1%"}), "", nil)
	if want := plainBody + "\n\n" + referencesSection + "\n- [references/rates.md](references/rates.md)\n"; got.Body != want {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func TestFilesOutsideTheShippedFoldersGetNoSection(t *testing.T) {
	got := assembleDraft(draftWith(plainBody, GeneratedFile{Path: "reference/rates.md", Content: "1%"}, GeneratedFile{Path: "scripts/sub/a.py", Content: "1"}), "", nil)
	if got.Body != plainBody {
		t.Fatalf("body = %q, want it unchanged", got.Body)
	}
}

func TestOutputCapsAreReadFromTheBriefAndCriteria(t *testing.T) {
	for text, want := range map[string][]string{
		"整封信不超過 3 句話":                    {"--max-sentences 3"},
		"輸出總字數不超過 35 個字":                 {"--max-chars 35"},
		"列出最多 5 項":                       {"--max-items 5"},
		"30 字以內回覆":                       {"--max-chars 30"},
		"最多 3 句，且不超過 2 句":                {"--max-sentences 2"},
		"at most 4 sentences":            {"--max-sentences 4"},
		"不超過　３　句":                        {"--max-sentences 3"},
		"寫一封信給客戶":                        nil,
		"每項說明不超過 30 字":                   nil,
		"列出最多 5 項，每項不超過 30 字":            {"--max-items 5"},
		"at most 2 sentences per bullet": nil,
		"每段最多 2 句\n整封信不超過 6 句":           {"--max-sentences 6"},
		"不超過 3 句，最多 40 字，最多 5 點":         {"--max-sentences 3", "--max-chars 40", "--max-items 5"},
	} {
		if got := outputCapFlags(text); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: flags = %v, want %v", text, got, want)
		}
	}
}

func TestACappedRequestGetsTheOutputCheckAndThePlatformScript(t *testing.T) {
	got := assembleDraft(draftWith(plainBody), "", []string{"整封信不超過 3 句話。"})
	if !strings.Contains(got.Body, "`python scripts/check_output.py --max-sentences 3 answer.txt`") {
		t.Fatalf("body = %q, want the output check with the criterion's cap", got.Body)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "scripts/check_output.py" {
		t.Fatalf("files = %v, want the platform's check script", got.Files)
	}
}

func TestACappedRequestWhoseBodyAlreadyChecksIsLeftAlone(t *testing.T) {
	body := plainBody + " Run `python scripts/check_output.py --max-chars 35 answer.txt`."
	got := assembleDraft(draftWith(body), "簡訊不超過 35 個字", nil)
	if got.Body != body {
		t.Fatalf("body = %q, want it unchanged", got.Body)
	}
}

func TestAssemblingAnAssembledDraftChangesNothing(t *testing.T) {
	once := assembleDraft(draftWith(plainBody, inputSchema, GeneratedFile{Path: "scripts/fee.py", Content: "1"}, GeneratedFile{Path: "references/a.md", Content: "2"}), "最多 3 句", nil)
	if twice := assembleDraft(once, "最多 3 句", nil); !reflect.DeepEqual(twice, once) {
		t.Fatalf("a second assembly changed the draft:\n%q\n%q", once.Body, twice.Body)
	}
}

func TestTheReplysDraftIsAssembledAgainstTheSessionsBrief(t *testing.T) {
	r := &StepResult{Message: "ok", Draft: &GeneratedSkill{Body: plainBody}}
	normalizeReply(r, Snapshot{Brief: "簡訊不超過 35 個字"})
	if !strings.Contains(r.Draft.Body, "check_output.py --max-chars 35 answer.txt") {
		t.Fatalf("body = %q, want the brief's cap checked", r.Draft.Body)
	}
}
