package eval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const (
	MaxCheckNodes      = 50
	MaxCheckDepth      = 8
	MaxCheckValues     = 20
	MaxCheckValueBytes = 200
	CheckItem          = "{item}"
)

const resultPending = "pending"

const (
	CategoryGrader    = "grader"
	CategoryComposite = "composite"
)

var ErrInvalidCheck = errors.New("eval: invalid check")

type Bounds struct{ Min, Max int }

type CheckKind struct {
	Kind        string
	Category    string
	Label       string
	Description string
	Values      Bounds
	Children    Bounds
	AtLeast     bool
}

type outcome struct {
	result   string
	reason   string
	judged   bool
	evidence []EvidenceRef
}

type checkScope struct {
	m     material
	judge *CriterionResult
	item  string
	steps *[]CheckStep
}

type CheckStep struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Result string `json:"result"`
	Reason string `json:"reason"`
}

type checkNode interface {
	kind() CheckKind
	evaluate(s checkScope, n testlab.Check, path string) outcome
}

var checkNodeList = []checkNode{
	containsValuesNode{}, asksNode{}, doesNotAskNode{}, judgeNode{},
	sequenceNode{}, allNode{}, anyNode{}, ifNode{}, eachNode{},
}

var checkNodes = registerCheckNodes(checkNodeList)

func registerCheckNodes(nodes []checkNode) map[string]checkNode {
	registry := make(map[string]checkNode, len(nodes))
	for _, n := range nodes {
		registry[n.kind().Kind] = n
	}
	return registry
}

func CheckKinds() []CheckKind {
	kinds := make([]CheckKind, 0, len(checkNodeList))
	for _, n := range checkNodeList {
		kinds = append(kinds, n.kind())
	}
	return kinds
}

func ValidateCheck(c *testlab.Check) (*testlab.Check, error) {
	if c == nil {
		return nil, nil
	}
	count := 0
	normalized, err := validateNode(*c, 1, &count)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func validateNode(c testlab.Check, depth int, count *int) (testlab.Check, error) {
	*count++
	if *count > MaxCheckNodes {
		return testlab.Check{}, fmt.Errorf("%w: 一個檢查最多 %d 個節點", ErrInvalidCheck, MaxCheckNodes)
	}
	if depth > MaxCheckDepth {
		return testlab.Check{}, fmt.Errorf("%w: 檢查最多 %d 層", ErrInvalidCheck, MaxCheckDepth)
	}
	node, known := checkNodes[c.Kind]
	if !known {
		return testlab.Check{}, fmt.Errorf("%w: 不認得的檢查方式 %q", ErrInvalidCheck, c.Kind)
	}
	k := node.kind()
	values, err := normalizedValues(c.Values)
	if err != nil {
		return testlab.Check{}, err
	}
	if !within(len(values), k.Values) {
		return testlab.Check{}, fmt.Errorf("%w: %s 的值要 %d 到 %d 個", ErrInvalidCheck, c.Kind, k.Values.Min, k.Values.Max)
	}
	if !within(len(c.Children), k.Children) {
		return testlab.Check{}, fmt.Errorf("%w: %s 要 %d 到 %d 個子節點", ErrInvalidCheck, c.Kind, k.Children.Min, k.Children.Max)
	}
	if !k.AtLeast && c.AtLeast != 0 || k.AtLeast && (c.AtLeast < 0 || c.AtLeast > len(values)) {
		return testlab.Check{}, fmt.Errorf("%w: %s 的 at_least 不合法", ErrInvalidCheck, c.Kind)
	}
	out := testlab.Check{Kind: c.Kind, Values: values, AtLeast: c.AtLeast}
	for _, child := range c.Children {
		normalized, err := validateNode(child, depth+1, count)
		if err != nil {
			return testlab.Check{}, err
		}
		out.Children = append(out.Children, normalized)
	}
	return out, nil
}

func normalizedValues(raw []string) ([]string, error) {
	var values []string
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, fmt.Errorf("%w: 檢查的值不能空白", ErrInvalidCheck)
		}
		if len(v) > MaxCheckValueBytes {
			return nil, fmt.Errorf("%w: 每個檢查的值最多 %d bytes", ErrInvalidCheck, MaxCheckValueBytes)
		}
		values = append(values, v)
	}
	return values, nil
}

func within(n int, b Bounds) bool { return n >= b.Min && n <= b.Max }

func evaluateCheck(s checkScope, n testlab.Check, path string) outcome {
	node, known := checkNodes[n.Kind]
	if !known {
		return outcome{result: ResultUndetermined, reason: fmt.Sprintf("the check kind %q is not one this platform knows", n.Kind)}
	}
	o := node.evaluate(s, n, path)
	if s.steps != nil && o.result != resultPending {
		*s.steps = append(*s.steps, CheckStep{Path: path, Kind: n.Kind, Result: o.result, Reason: o.reason})
	}
	return o
}

func childPath(path string, i int) string { return fmt.Sprintf("%s.%d", path, i) }

func (s checkScope) withItem(values []string) []string {
	if s.item == "" {
		return values
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ReplaceAll(v, CheckItem, s.item)
	}
	return out
}

type ruleLeaf struct {
	info  CheckKind
	grade ruleGrader
}

func (r ruleLeaf) kind() CheckKind { return r.info }

func (r ruleLeaf) evaluate(s checkScope, n testlab.Check, _ string) outcome {
	result, reason := r.grade(s.m, s.withItem(n.Values))
	if result == ResultPassed && !s.m.evidenceComplete() {
		result = ResultUndetermined
		reason += "; the run's evidence is incomplete, so this is not recorded as passed"
	}
	return outcome{result: result, reason: reason}
}

type containsValuesNode struct{}

func (containsValuesNode) kind() CheckKind {
	return CheckKind{Kind: "contains_values", Category: CategoryGrader, Label: "回答提到指定的值",
		Description: "最終回答提到每個值；數字與日期認常見寫法", Values: Bounds{1, MaxCheckValues}}
}

func (n containsValuesNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), gradeContainsValues}.evaluate(s, c, path)
}

type asksNode struct{}

func (asksNode) kind() CheckKind {
	return CheckKind{Kind: "asks", Category: CategoryGrader, Label: "對話有提問",
		Description: "整段對話向使用者提過問題；可指定問題要涵蓋的事", Values: Bounds{0, MaxCheckValues}}
}

func (n asksNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), gradeAsks}.evaluate(s, c, path)
}

type doesNotAskNode struct{}

func (doesNotAskNode) kind() CheckKind {
	return CheckKind{Kind: "does_not_ask", Category: CategoryGrader, Label: "不提問就答完",
		Description: "Run 成功，且整段對話沒有向使用者提問"}
}

func (n doesNotAskNode) evaluate(s checkScope, c testlab.Check, path string) outcome {
	return ruleLeaf{n.kind(), gradeDoesNotAsk}.evaluate(s, c, path)
}

type judgeNode struct{}

func (judgeNode) kind() CheckKind {
	return CheckKind{Kind: "judge", Category: CategoryGrader, Label: "評審判定",
		Description: "交給評審模型依這條驗收條件的文字判定"}
}

func (judgeNode) evaluate(s checkScope, _ testlab.Check, _ string) outcome {
	if s.judge == nil {
		return outcome{result: resultPending, reason: "waiting for the judge"}
	}
	return outcome{result: s.judge.Result, reason: s.judge.Reason, judged: true, evidence: s.judge.Evidence}
}

type sequenceNode struct{}

func (sequenceNode) kind() CheckKind {
	return CheckKind{Kind: "sequence", Category: CategoryComposite, Label: "串聯",
		Description: "依序執行，前一步沒通過就停下，後面的步驟不執行", Children: Bounds{1, MaxCheckNodes}}
}

func (sequenceNode) evaluate(s checkScope, n testlab.Check, path string) outcome {
	var merged outcome
	for i, child := range n.Children {
		o := evaluateCheck(s, child, childPath(path, i))
		merged = merged.joinedWith(o)
		if o.result != ResultPassed {
			merged.result, merged.reason = o.result, fmt.Sprintf("step %d stopped the sequence: %s", i+1, o.reason)
			return merged
		}
	}
	merged.result, merged.reason = ResultPassed, fmt.Sprintf("all %d steps passed", len(n.Children))
	return merged
}

type allNode struct{}

func (allNode) kind() CheckKind {
	return CheckKind{Kind: "all", Category: CategoryComposite, Label: "並聯（全部通過）",
		Description: "每個分支都執行，全部通過才算通過", Children: Bounds{1, MaxCheckNodes}}
}

func (allNode) evaluate(s checkScope, n testlab.Check, path string) outcome {
	return combine(s, n, path, conjunction)
}

type anyNode struct{}

func (anyNode) kind() CheckKind {
	return CheckKind{Kind: "any", Category: CategoryComposite, Label: "並聯（任一通過）",
		Description: "每個分支都執行，任一通過就算通過", Children: Bounds{1, MaxCheckNodes}}
}

func (anyNode) evaluate(s checkScope, n testlab.Check, path string) outcome {
	return combine(s, n, path, disjunction)
}

func combine(s checkScope, n testlab.Check, path string, rule func(counts) string) outcome {
	var merged outcome
	var c counts
	for i, child := range n.Children {
		o := evaluateCheck(s, child, childPath(path, i))
		merged = merged.joinedWith(o)
		c.add(o.result)
	}
	merged.result = rule(c)
	merged.reason = fmt.Sprintf("%d of %d branches passed", c.passed, len(n.Children))
	return merged
}

type ifNode struct{}

const ifElseBranch = 2

func (ifNode) kind() CheckKind {
	return CheckKind{Kind: "if", Category: CategoryComposite, Label: "條件",
		Description: "第一個子節點是條件：通過走第二個，沒通過走第三個（沒有第三個就算通過）",
		Children:    Bounds{2, 3}}
}

func (ifNode) evaluate(s checkScope, n testlab.Check, path string) outcome {
	condition := evaluateCheck(s, n.Children[0], childPath(path, 0))
	switch condition.result {
	case ResultPassed:
		o := evaluateCheck(s, n.Children[1], childPath(path, 1))
		return condition.joinedWith(o).as(o.result, "the condition held: "+o.reason)
	case ResultFailed:
		if len(n.Children) <= ifElseBranch {
			return condition.as(ResultPassed, "the condition did not hold, so nothing else was required")
		}
		o := evaluateCheck(s, n.Children[ifElseBranch], childPath(path, ifElseBranch))
		return condition.joinedWith(o).as(o.result, "the condition did not hold: "+o.reason)
	default:
		return condition.as(condition.result, "the condition could not be decided: "+condition.reason)
	}
}

type eachNode struct{}

func (eachNode) kind() CheckKind {
	return CheckKind{Kind: "each", Category: CategoryComposite, Label: "Fan-Out",
		Description: "對每個值各執行一次子節點（子節點的值可用 {item} 代入），至少 at_least 個通過才算通過；at_least 為 0 表示全部",
		Values:      Bounds{1, MaxCheckValues}, Children: Bounds{1, 1}, AtLeast: true}
}

func (eachNode) evaluate(s checkScope, n testlab.Check, path string) outcome {
	needed := n.AtLeast
	if needed == 0 {
		needed = len(n.Values)
	}
	var merged outcome
	var c counts
	for i, item := range n.Values {
		scoped := s
		scoped.item = item
		o := evaluateCheck(scoped, n.Children[0], childPath(path, i))
		merged = merged.joinedWith(o)
		c.add(o.result)
	}
	merged.result = c.atLeast(needed, len(n.Values))
	merged.reason = fmt.Sprintf("%d of %d items passed, %d needed", c.passed, len(n.Values), needed)
	return merged
}

type counts struct{ passed, failed, undetermined, pending int }

func (c *counts) add(result string) {
	switch result {
	case ResultPassed:
		c.passed++
	case ResultFailed:
		c.failed++
	case resultPending:
		c.pending++
	default:
		c.undetermined++
	}
}

func conjunction(c counts) string {
	return c.atLeast(c.passed+c.failed+c.undetermined+c.pending, c.passed+c.failed+c.undetermined+c.pending)
}

func disjunction(c counts) string { return c.atLeast(1, c.passed+c.failed+c.undetermined+c.pending) }

// Kleene three-valued k-of-n: decided as soon as the passes reach k or the
// failures make k unreachable; otherwise a pending branch outranks an
// undetermined one, because the judge may still decide it either way.
func (c counts) atLeast(k, n int) string {
	switch {
	case c.passed >= k:
		return ResultPassed
	case n-c.failed < k:
		return ResultFailed
	case c.pending > 0:
		return resultPending
	default:
		return ResultUndetermined
	}
}

func (o outcome) joinedWith(other outcome) outcome {
	o.judged = o.judged || other.judged
	o.evidence = append(o.evidence, other.evidence...)
	return o
}

func (o outcome) as(result, reason string) outcome {
	o.result, o.reason = result, reason
	return o
}
