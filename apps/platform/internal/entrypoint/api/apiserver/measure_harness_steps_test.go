package apiserver_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func mustDecodeJSON[T any](t *testing.T, raw string) T {
	t.Helper()
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func strp(s string) *string { return &s }

func boolp(b bool) *bool { return &b }

func floatp(f float64) *float64 { return &f }

func anchorHistoricalInputs() []string {
	return []string{
		"### 語言 zh\n| BM25 | 6/30 |\n### 語言 en\n| BM25 | 14/18 (78%) |\n### 語言 fr\n",
		"### 語言 zh\n| BM25 | 14/18 (78%) |\n",
		"### 語言 en\n| BM25 | 13/18 (72%) |\n### 語言 fr\n| BM25 | 14/18 (78%) |\n",
		"",
	}
}

func anchorIntentRows(t *testing.T) []goldenIntentRow {
	return mustDecodeJSON[[]goldenIntentRow](t, `[
		{"query":"a","response":{"valid":true}},
		{"query":"b","response":{"valid":false}},
		{"query":"c","response":null},
		{"query":"d","response":{"valid":false}},
		{"query":"d","response":{"valid":true}},
		{"query":"a","response":null}
	]`)
}

func anchorFrontInputs() []string {
	return []string{
		"name: x\r\ndescription: y\r\n---\r\nbody",
		"no fence here",
		"---\nname: x\n---\nbody",
		"",
	}
}

func anchorExamples() []goldenTaskExample {
	return []goldenTaskExample{{Zh: " 中文 ", En: "en"}, {Zh: "", En: "  "}, {Zh: "zh2", En: ""}}
}

func anchorIntentRequests(t *testing.T) map[string]intentGatewayRequest {
	return map[string]intentGatewayRequest{
		"keeps provenance":   mustDecodeJSON[intentGatewayRequest](t, `{"metadata":{"prompt_version":"search-intent/v2"},"messages":[{"content":"system"},{"content":"任務：請將收支資料轉成報告"}]}`),
		"older prompt":       mustDecodeJSON[intentGatewayRequest](t, `{"metadata":{"prompt_version":"search-intent/v1"},"messages":[{"content":"system"},{"content":"請將收支資料轉成報告"}]}`),
		"one message":        mustDecodeJSON[intentGatewayRequest](t, `{"metadata":{"prompt_version":"search-intent/v2"},"messages":[{"content":"請將收支資料轉成報告"}]}`),
		"task lost":          mustDecodeJSON[intentGatewayRequest](t, `{"metadata":{"prompt_version":"search-intent/v2"},"messages":[{"content":"system"},{"content":"其他任務"}]}`),
		"three messages":     mustDecodeJSON[intentGatewayRequest](t, `{"metadata":{"prompt_version":"search-intent/v2"},"messages":[{"content":"system"},{"content":"請將收支資料轉成報告"},{"content":"x"}]}`),
		"no prompt metadata": mustDecodeJSON[intentGatewayRequest](t, `{"messages":[{"content":"system"},{"content":"請將收支資料轉成報告"}]}`),
	}
}

type anchorCostCase struct {
	rows intentCostRows
	want int64
}

func anchorCostCases() []anchorCostCase {
	return []anchorCostCase{
		{intentCostRows{1, 1000, 10, 20, true}, 1},
		{intentCostRows{0, 0, 0, 0, true}, 0},
		{intentCostRows{1, 999, 10, 20, true}, 1},
		{intentCostRows{1, 1000, 11, 20, true}, 1},
		{intentCostRows{1, 1000, 10, 21, true}, 1},
		{intentCostRows{1, 1000, 10, 20, false}, 1},
		{intentCostRows{2, 2000, 20, 40, true}, 1},
		{intentCostRows{1, 1000, 10, 20, true}, 0},
		{intentCostRows{0, 0, 0, 0, true}, 1},
	}
}

func ledgerIntent() map[string]*string {
	return map[string]*string{"input": strp("收支資料"), "output": strp("報告"), "tools": nil, "data": nil, "environment": nil}
}

func anchorLedgerCases() map[string]catalog.SearchInterpretation {
	withTools := ledgerIntent()
	withTools["tools"] = strp("excel")
	fourFields := ledgerIntent()
	delete(fourFields, "environment")
	wrongInput := ledgerIntent()
	wrongInput["input"] = strp("收支")
	noOutput := ledgerIntent()
	noOutput["output"] = nil
	withData := ledgerIntent()
	withData["data"] = strp("x")
	withEnvironment := ledgerIntent()
	withEnvironment["environment"] = strp("x")
	return map[string]catalog.SearchInterpretation{
		"the ledger answer":        {Intent: ledgerIntent(), PromptVersion: "search-intent/v2"},
		"an invented tool":         {Intent: withTools, PromptVersion: "search-intent/v2"},
		"four fields":              {Intent: fourFields, PromptVersion: "search-intent/v2"},
		"a shorter input":          {Intent: wrongInput, PromptVersion: "search-intent/v2"},
		"no output":                {Intent: noOutput, PromptVersion: "search-intent/v2"},
		"an invented data field":   {Intent: withData, PromptVersion: "search-intent/v2"},
		"an invented environment":  {Intent: withEnvironment, PromptVersion: "search-intent/v2"},
		"an older prompt":          {Intent: ledgerIntent(), PromptVersion: "search-intent/v1"},
		"no interpretation at all": {},
	}
}

func liveBody() liveSearchBody {
	return liveSearchBody{
		Query:          "請將收支資料轉成報告",
		Interpretation: catalog.SearchInterpretation{Status: "analyzed", Model: "m", PromptVersion: "search-intent/v2", Intent: ledgerIntent()},
	}
}

func anchorLiveBodies() map[string]liveSearchBody {
	degraded := liveBody()
	degraded.Degraded = true
	noModel := liveBody()
	noModel.Interpretation.Model = ""
	otherQuery := liveBody()
	otherQuery.Query = "其他"
	fourFields := liveBody()
	fourFields.Interpretation.Intent = map[string]*string{"input": nil, "output": nil, "tools": nil, "data": nil}
	fallback := liveBody()
	fallback.Interpretation.Status = "fallback"
	olderPrompt := liveBody()
	olderPrompt.Interpretation.PromptVersion = "search-intent/v1"
	return map[string]liveSearchBody{
		"analyzed": liveBody(), "degraded": degraded, "no model": noModel, "other query": otherQuery,
		"four fields": fourFields, "fallback": fallback, "older prompt": olderPrompt,
	}
}

func anchorIntents() map[string]map[string]*string {
	all := func() map[string]*string {
		return map[string]*string{"input": nil, "output": nil, "tools": nil, "data": nil, "environment": nil}
	}
	quoted := all()
	quoted["input"], quoted["output"] = strp("收支資料"), strp("報告")
	invented := all()
	invented["input"], invented["output"] = strp("收支資料"), strp("報表")
	missing := all()
	delete(missing, "data")
	blank := all()
	blank["tools"] = strp("  ")
	twoBad := all()
	twoBad["input"], twoBad["environment"] = strp("雲端"), strp("  ")
	return map[string]map[string]*string{
		"all null": all(), "quoted": quoted, "invented output": invented,
		"missing data": missing, "blank tools": blank, "two bad fields": twoBad,
	}
}

func anchorFindings() []skillpkg.Finding {
	return []skillpkg.Finding{
		{Severity: skillpkg.SeverityError, Code: "a"},
		{Severity: skillpkg.SeverityWarning, Code: "b"},
		{Severity: skillpkg.SeverityError, Code: "c"},
		{Severity: skillpkg.SeverityInfo, Code: "d"},
	}
}

func anchorEvaluation(t *testing.T) evaluationBody {
	return mustDecodeJSON[evaluationBody](t, `{"criterion_results":[
		{"text":"a","result":"met"},{"text":"b","result":"not_met"},{"text":"a","result":"unknown"}
	]}`)
}

func anchorUsages() map[string]map[string]any {
	return map[string]map[string]any{
		"reported by the gateway": {"cost_source": "gateway", "cost_usd": 0.5, "input_tokens": 3.0},
		"a local estimate":        {"cost_source": "local", "cost_usd": 0.0, "input_tokens": "3"},
		"nothing reported":        {},
		"negative tokens":         {"cost_source": "gateway", "cost_usd": 0.25, "input_tokens": -1.0},
	}
}

type anchorTurn struct {
	call           int32
	system, prompt string
}

func anchorTurns() []anchorTurn {
	return []anchorTurn{
		{1, "Current phase: understand", ""},
		{1, "Current phase: compose", ""},
		{2, "Current phase: compose", ""},
		{2, "x", ""},
		{3, "Current phase: revise", "finding description-missing"},
		{3, "Current phase: revise", "nothing"},
		{3, "x", "description-missing"},
		{4, "Current phase: review", `{"blocked":false}`},
		{4, "Current phase: review", `{"blocked":true}`},
		{4, "x", `{"blocked":false}`},
		{5, "", ""},
		{0, "", ""},
	}
}

func anchorMixedCorpus(t *testing.T, dir string) modesCorpus {
	t.Helper()
	var corpus modesCorpus
	for i := 0; i < 10; i++ {
		r := modesReference{ID: fmt.Sprintf("r%d", i), Description: fmt.Sprintf("d%d", i)}
		r.Reference.SkillMD = fmt.Sprintf("md%d", i)
		if i%3 == 0 {
			r.Holdout = []holdoutCase{{Name: fmt.Sprintf("h%d", i)}}
		}
		corpus.Reference = append(corpus.Reference, r)
	}
	for i := 0; i < 5; i++ {
		d := modesDiagram{ID: fmt.Sprintf("g%d", i), Media: []string{"jpg", "png"}[i%2]}
		for j := 0; j < i; j++ {
			d.Nodes = append(d.Nodes, struct {
				Label string `json:"label"`
				Key   string `json:"key"`
			}{Label: fmt.Sprintf("n%d.%d", i, j)})
		}
		if err := os.WriteFile(filepath.Join(dir, d.ID+"."+d.Media), []byte("img-"+d.ID), 0o600); err != nil {
			t.Fatal(err)
		}
		corpus.Diagram = append(corpus.Diagram, d)
	}
	return corpus
}

func describeTasks(tasks []measureTask) string {
	var parts []string
	for _, task := range tasks {
		diagram := "none"
		if task.Diagram != nil {
			diagram = task.Diagram.MediaType + ":" + string(task.Diagram.Data)
		}
		parts = append(parts, fmt.Sprintf("%s/%s/%s/%s/holdout=%d/%s/nodes=%q", task.ID, task.Kind, task.Description, task.ReferenceMD, len(task.Holdout), diagram, task.DiagramNodes))
	}
	return strings.Join(parts, " ")
}

func anchorSummaryRows() ([]sessionRow, []singleShotRow) {
	interactive := []sessionRow{
		{Kind: "text", Draft: true, Met: boolp(true), MetRound: 1, CostUSD: floatp(0.3), SecondsPerCall: []float64{1, 2}, HoldoutMet: boolp(true), CriteriaChangedBeforeMet: boolp(true)},
		{Kind: "diagram", Draft: true, Blocked: true, Met: boolp(false), SecondsPerCall: []float64{3}},
		{Kind: "text", CostUSD: floatp(0.1), SecondsPerCall: []float64{4, 5, 6}},
		{Kind: "reference", Draft: true, Met: boolp(false), MetRound: 2, CostUSD: floatp(0.2), HoldoutMet: boolp(false), CriteriaChangedBeforeMet: boolp(false)},
		{Kind: "diagram", Draft: true, Met: boolp(true), MetRound: 1, SecondsPerCall: []float64{7, 8}},
	}
	singleShot := []singleShotRow{{Generated: true}, {Generated: false}, {Generated: true}}
	return interactive, singleShot
}

func TestTheGoldenBaselineIsFoundOnlyInTheEnglishSection(t *testing.T) {
	want := []bool{true, false, false, false}
	for i, results := range anchorHistoricalInputs() {
		if got := historicalEnglishBaselinePresent(results); got != want[i] {
			t.Errorf("historicalEnglishBaselinePresent(%q) = %v, want %v", results, got, want[i])
		}
	}
}

func TestTheGoldenReplayCountsQueriesAndExpectsAnalysisOnlyForAValidRow(t *testing.T) {
	replay := goldenIntentReplay{Rows: anchorIntentRows(t)}
	for _, tc := range []struct {
		query    string
		matches  int
		expected string
	}{
		{"a", 2, "analyzed"}, {"b", 1, "fallback"}, {"c", 1, "fallback"}, {"d", 2, "analyzed"}, {"e", 0, "fallback"},
	} {
		if got := replay.matches(tc.query); got != tc.matches {
			t.Errorf("matches(%q) = %d, want %d", tc.query, got, tc.matches)
		}
		if got := replay.expectedStatus(tc.query); got != tc.expected {
			t.Errorf("expectedStatus(%q) = %q, want %q", tc.query, got, tc.expected)
		}
	}
}

func TestAGoldenFileReadsItsSourceFromTheCorpusUnlessItIsPoison(t *testing.T) {
	root, file := "root", filepath.Join("root", "corpus_enriched", "writing", "draft.json")
	for _, tc := range []struct {
		name                     string
		poison                   map[string]string
		stem, category, wantPath string
	}{
		{"clean", map[string]string{}, "draft", "writing", filepath.Join("root", "corpus", "writing", "draft.md")},
		{"poison", map[string]string{"draft": "documents"}, "draft", "documents", filepath.Join("root", "corpus_enriched", "poison", "draft.md")},
	} {
		stem, category, source := goldenSource(root, file, tc.poison)
		if stem != tc.stem || category != tc.category || source != tc.wantPath {
			t.Errorf("%s: goldenSource = %q %q %q, want %q %q %q", tc.name, stem, category, source, tc.stem, tc.category, tc.wantPath)
		}
	}
}

func TestAGoldenFrontmatterIsEverythingBeforeTheFirstFence(t *testing.T) {
	want := []struct {
		head string
		ok   bool
	}{{"name: x\ndescription: y", true}, {"", false}, {"---\nname: x", true}, {"", false}}
	for i, raw := range anchorFrontInputs() {
		head, ok := goldenFrontmatterBlock(raw)
		if head != want[i].head || ok != want[i].ok {
			t.Errorf("goldenFrontmatterBlock(%q) = %q %v, want %q %v", raw, head, ok, want[i].head, want[i].ok)
		}
	}
}

func TestGoldenTaskExamplesKeepTrimmedNonBlankTextInOrder(t *testing.T) {
	if got := goldenTaskExamples(anchorExamples()); fmt.Sprintf("%#v", got) != `[]string{"中文", "en", "zh2"}` {
		t.Errorf("goldenTaskExamples = %#v", got)
	}
	if got := goldenTaskExamples(nil); got != nil {
		t.Errorf("goldenTaskExamples(nil) = %#v, want nil", got)
	}
}

func TestTheEmbeddingTextJoinsSummaryExamplesAndFourTagBuckets(t *testing.T) {
	for _, c := range []struct {
		name, summary, enriched, examples, tags, want string
	}{
		{"examples and tags", "s", "e", "e1\ne2", `{"inputs":["i"],"tools":["t1","t2"],"other":["x"]}`, "n: e\ne1\ne2\ni t1 t2"},
		{"summary only", "s", "e", "", "", "n: e"},
		{"buckets in fixed order", "s", "e", "", `{"dependencies":["d"],"outputs":["o"]}`, "n: e\no d"},
		{"no enriched summary falls back", "s", "", "", "", "n: s"},
		{"unreadable tags are left out", "s", "e", "", `{"inputs":`, "n: e"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ingest.EmbeddingText("n", c.summary, c.enriched, c.examples, []byte(c.tags)); got != c.want {
				t.Errorf("EmbeddingText = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAnAbsentGoldIsTheFirstRelevantIDNotInTheCorpus(t *testing.T) {
	ids := map[string]string{"x": "a", "y": "b"}
	for _, tc := range []struct {
		relevant []string
		gold     string
		absent   bool
	}{
		{[]string{"a", "b"}, "", false}, {[]string{"a", "c", "d"}, "c", true}, {[]string{}, "", false},
	} {
		if gold, absent := absentGold(tc.relevant, ids); gold != tc.gold || absent != tc.absent {
			t.Errorf("absentGold(%v) = %q %v, want %q %v", tc.relevant, gold, absent, tc.gold, tc.absent)
		}
	}
}

func TestPoisonCountsOnlyInsideTheTopThree(t *testing.T) {
	poison := map[string]string{"p": "data"}
	for _, tc := range []struct {
		results []string
		want    bool
	}{
		{[]string{"a", "b", "p"}, true}, {[]string{"a", "b", "c", "p"}, false}, {nil, false}, {[]string{"p"}, true},
	} {
		if got := poisonInTopThree(tc.results, poison); got != tc.want {
			t.Errorf("poisonInTopThree(%v) = %v, want %v", tc.results, got, tc.want)
		}
	}
}

func TestSearchResultsContainOnlyTheIDsReturned(t *testing.T) {
	results := []searchResultRef{{ID: "a"}, {ID: "b"}}
	if !searchResultsContain(results, "b") || searchResultsContain(results, "c") || searchResultsContain(nil, "a") {
		t.Error("searchResultsContain disagrees with the returned IDs")
	}
}

func TestAnIntentRequestKeepsProvenanceOnlyWithThePromptVersionAndTheTaskInTheSecondOfTwoMessages(t *testing.T) {
	want := map[string]bool{
		"keeps provenance": true, "older prompt": false, "one message": false,
		"task lost": false, "three messages": false, "no prompt metadata": false,
	}
	for name, request := range anchorIntentRequests(t) {
		if got := intentRequestKeepsProvenance(request, "請將收支資料轉成報告"); got != want[name] {
			t.Errorf("%s: intentRequestKeepsProvenance = %v, want %v", name, got, want[name])
		}
	}
}

func TestIntentCostRowsMatchOnlyTheExactPerCallAmounts(t *testing.T) {
	want := []bool{true, true, false, false, false, false, false, false, false}
	for i, c := range anchorCostCases() {
		if got := c.rows.matches(c.want); got != want[i] {
			t.Errorf("%+v matches(%d) = %v, want %v", c.rows, c.want, got, want[i])
		}
	}
}

func TestOnlyTheLedgerInterpretationCountsAsAnalyzed(t *testing.T) {
	for name, i := range anchorLedgerCases() {
		if got := analyzedLedgerInterpretation(i); got != (name == "the ledger answer") {
			t.Errorf("%s: analyzedLedgerInterpretation = %v", name, got)
		}
	}
}

func TestALiveSearchCountsAsAnalyzedOnlyWhenEveryFieldAgrees(t *testing.T) {
	for name, body := range anchorLiveBodies() {
		if got := body.analyzedFrom("請將收支資料轉成報告"); got != (name == "analyzed") {
			t.Errorf("%s: analyzedFrom = %v", name, got)
		}
	}
}

func TestAnInventedIntentFieldIsTheFirstMissingBlankOrUnquotedOne(t *testing.T) {
	want := map[string]string{
		"all null": "", "quoted": "", "invented output": "output",
		"missing data": "data", "blank tools": "tools", "two bad fields": "input",
	}
	for name, intent := range anchorIntents() {
		field, invented := inventedIntentField(intent, "請將收支資料轉成報告")
		if field != want[name] || invented != (want[name] != "") {
			t.Errorf("%s: inventedIntentField = %q %v, want %q", name, field, invented, want[name])
		}
	}
}

func TestOnlyAJpgDiagramIsSentAsJPEG(t *testing.T) {
	for ext, want := range map[string]string{"jpg": "image/jpeg", "png": "image/png", "jpeg": "image/png", "": "image/png"} {
		if got := diagramMediaType(ext); got != want {
			t.Errorf("diagramMediaType(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestFindingCodesKeepOrderAndErrorCodesKeepOnlyErrors(t *testing.T) {
	if got := fmt.Sprintf("%#v", errorFindingCodes(anchorFindings())); got != `[]string{"a", "c"}` {
		t.Errorf("errorFindingCodes = %s", got)
	}
	if got := fmt.Sprintf("%#v", findingCodes(anchorFindings())); got != `[]string{"a", "b", "c", "d"}` {
		t.Errorf("findingCodes = %s", got)
	}
	if errorFindingCodes(nil) != nil || findingCodes(nil) != nil {
		t.Error("no findings must stay nil so the row omits them")
	}
}

func TestCountContainedCountsEveryNeedleFoundIncludingTheEmptyOne(t *testing.T) {
	if got := countContained("abc", []string{"a", "x", "bc", ""}); got != 3 {
		t.Errorf("countContained = %d, want 3", got)
	}
}

func TestAReferenceBodyIsWhatFollowsTheFrontmatter(t *testing.T) {
	for md, want := range map[string]string{"---\nname: x\n---\nbody\n": "body\n", "plain": "", "---\nname: x\n---\n": "", "---\n---\nbody": ""} {
		if got := referenceSkillBody(md); got != want {
			t.Errorf("referenceSkillBody(%q) = %q, want %q", md, got, want)
		}
	}
}

func TestAGenerationCostIsUnpricedOrSixDecimalDollars(t *testing.T) {
	cost := 0.1234567
	if got := generationCostText(nil); got != "unpriced" {
		t.Errorf("generationCostText(nil) = %q", got)
	}
	if got := generationCostText(&cost); got != "US$0.123457" {
		t.Errorf("generationCostText = %q", got)
	}
}

func TestCriteriaByTextKeepsTheLastResultPerCriterion(t *testing.T) {
	if got := fmt.Sprintf("%#v", criteriaByText(anchorEvaluation(t))); got != `map[string]string{"a":"unknown", "b":"not_met"}` {
		t.Errorf("criteriaByText = %s", got)
	}
	if got := criteriaByText(evaluationBody{}); got == nil || len(got) != 0 {
		t.Errorf("criteriaByText(empty) = %#v, want an empty map", got)
	}
}

func TestGatewayUsageFindingsNameEveryMissingGatewayFact(t *testing.T) {
	want := map[string]struct {
		cost     float64
		problems []string
	}{
		"reported by the gateway": {0.5, nil},
		"a local estimate": {0, []string{
			"cost_source = local, want gateway (a local estimate is not a cost)",
			"cost_usd = 0, want a positive number reported by the gateway",
			"input_tokens = 3, want the SDK's own count",
		}},
		"nothing reported": {0, []string{
			"cost_source = <nil>, want gateway (a local estimate is not a cost)",
			"cost_usd = <nil>, want a positive number reported by the gateway",
			"input_tokens = <nil>, want the SDK's own count",
		}},
		"negative tokens": {0.25, []string{"input_tokens = -1, want the SDK's own count"}},
	}
	for name, usage := range anchorUsages() {
		cost, problems := gatewayUsageFindings(usage)
		if fmt.Sprintf("$%.6f %#v", cost, problems) != fmt.Sprintf("$%.6f %#v", want[name].cost, want[name].problems) {
			t.Errorf("%s: gatewayUsageFindings = %v %#v, want %v %#v", name, cost, problems, want[name].cost, want[name].problems)
		}
	}
}

func TestTheDeliveryLagPercentileTruncatesItsIndex(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if got := deliveryLagAt(sorted, 0.5); got != 3 {
		t.Errorf("p50 = %v, want 3", got)
	}
	if got := deliveryLagAt(sorted, 0.9); got != 4 {
		t.Errorf("p90 = %v, want 4", got)
	}
	if got := deliveryLagAt([]float64{7}, 0.9); got != 7 {
		t.Errorf("single p90 = %v, want 7", got)
	}
}

func TestTheScriptedCreationGatewayAnswersEachTurnAndFlagsTheWrongPhase(t *testing.T) {
	bad, good, review := map[string]any{"k": "bad"}, map[string]any{"k": "good"}, map[string]any{"k": "review"}
	script := creationTurnScript{bad: bad, good: good, review: review}
	confirm := mustJSON(t, creationDecision("confirm_brief", "請確認任務與成功條件。", ptr("整理輸入資料並依指定格式輸出摘要。"), nil))
	want := []struct{ decision, problem string }{
		{"confirm", ""},
		{"confirm", "first prompt did not use understand phase"},
		{"bad", ""},
		{"bad", "second prompt did not use compose phase"},
		{"good", ""},
		{"good", "revision prompt omitted Go finding: nothing"},
		{"good", "revision prompt omitted Go finding: description-missing"},
		{"review", ""},
		{"review", `review prompt omitted passing validation: {"blocked":true}`},
		{"review", `review prompt omitted passing validation: {"blocked":false}`},
		{"review", "unexpected model call 5"},
		{"review", "unexpected model call 0"},
	}
	for i, turn := range anchorTurns() {
		decision, problem := script.respond(turn.call, turn.system, turn.prompt)
		got := fmt.Sprint(decision["k"])
		if mustJSON(t, decision) == confirm {
			got = "confirm"
		}
		if got != want[i].decision || problem != want[i].problem {
			t.Errorf("turn %d %q: respond = %s %q, want %s %q", turn.call, turn.system, got, problem, want[i].decision, want[i].problem)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestTheCreationPythonPathPutsTheRepoSourcesBeforeTheInheritedPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	base := filepath.Join("root", "apps", "llm", "src") + sep + filepath.Join("root", "packages", "api-stub-py", "src")
	if got := creationPythonPath("root", ""); got != base {
		t.Errorf("creationPythonPath without inheritance = %q, want %q", got, base)
	}
	if got := creationPythonPath("root", "inh"); got != base+sep+"inh" {
		t.Errorf("creationPythonPath with inheritance = %q", got)
	}
}

func TestMeasureRoundsDefaultsToThreeUnlessGivenAPositiveInteger(t *testing.T) {
	for raw, want := range map[string]int{"": 3, "5": 5, "0": 3, "-1": 3, "1": 1, "x": 3} {
		if got := measureRounds(raw); got != want {
			t.Errorf("measureRounds(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestADiagramSessionOpensWithoutAMessage(t *testing.T) {
	for kind, want := range map[string]string{"diagram": "", "text": "desc", "reference": "desc"} {
		if got := initialCreationMessage(measureTask{Kind: kind, Description: "desc"}); got != want {
			t.Errorf("initialCreationMessage(%s) = %q, want %q", kind, got, want)
		}
	}
}

func TestADraftHashIsEmptyWithoutADraft(t *testing.T) {
	if got := draftHashOf(creation.View{}); got != "" {
		t.Errorf("draftHashOf(no draft) = %q", got)
	}
	if got := draftHashOf(creation.View{Snapshot: creation.Snapshot{Draft: &creation.Draft{ContentHash: "h"}}}); got != "h" {
		t.Errorf("draftHashOf = %q, want h", got)
	}
}

func TestMeasureTasksRunTextThenDiagramsThenReferencesAndFilterByKind(t *testing.T) {
	dir := t.TempDir()
	corpus := anchorMixedCorpus(t, dir)
	text := `r0/text/d0//holdout=1/none/nodes=[] r1/text/d1//holdout=0/none/nodes=[] r2/text/d2//holdout=0/none/nodes=[] r3/text/d3//holdout=1/none/nodes=[] r4/text/d4//holdout=0/none/nodes=[]`
	diagrams := `g0/diagram///holdout=0/image/jpeg:img-g0/nodes=[] g1/diagram///holdout=0/image/png:img-g1/nodes=["n1.0"] g2/diagram///holdout=0/image/jpeg:img-g2/nodes=["n2.0" "n2.1"] g3/diagram///holdout=0/image/png:img-g3/nodes=["n3.0" "n3.1" "n3.2"] g4/diagram///holdout=0/image/jpeg:img-g4/nodes=["n4.0" "n4.1" "n4.2" "n4.3"]`
	references := `r5/reference/d5/md5/holdout=0/none/nodes=[] r6/reference/d6/md6/holdout=1/none/nodes=[] r7/reference/d7/md7/holdout=0/none/nodes=[] r8/reference/d8/md8/holdout=0/none/nodes=[] r9/reference/d9/md9/holdout=1/none/nodes=[]`
	for only, want := range map[string]string{"": text + " " + diagrams + " " + references, "diagram": diagrams, "reference": references, "text": text} {
		if got := describeTasks(onlyMeasureTasks(mixedMeasureTasks(t, corpus, dir), only)); got != want {
			t.Errorf("only=%q:\n got %s\nwant %s", only, got, want)
		}
	}
	textOnly := `r0/text/d0//holdout=1/none/nodes=[] r1/text/d1//holdout=0/none/nodes=[] r2/text/d2//holdout=0/none/nodes=[]`
	if got := describeTasks(onlyMeasureTasks(textMeasureTasks(corpus.Reference[:3]), "")); got != textOnly {
		t.Errorf("text only:\n got %s\nwant %s", got, textOnly)
	}
}

func TestTheCreationMeasureSummaryCountsFormatJudgementAndLatency(t *testing.T) {
	interactive, singleShot := anchorSummaryRows()
	want := creationMeasureSummary{
		FormatPass: 5, CostMedian: 0.2, P50Seconds: 5, P95Seconds: 8,
		MetFirstCount: 1, MetCount: 2, MetOnChangedCriteria: 1, HoldoutMetCount: 1, HoldoutDenominator: 2,
		MetDenominator: 2, DiagramMetCount: 1, DiagramMetDenominator: 2,
	}
	if got := summarizeCreationMeasure(interactive, singleShot); got != want {
		t.Errorf("summarizeCreationMeasure = %+v, want %+v", got, want)
	}
}
