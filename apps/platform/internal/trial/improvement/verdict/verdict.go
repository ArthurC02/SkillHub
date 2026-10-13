package verdict

const (
	ResultPassed       = "passed"
	ResultFailed       = "failed"
	ResultUndetermined = "undetermined"
)

const (
	SourceModel = "model"
	SourceRule  = "rule"
)

type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type EvidenceRef struct {
	Kind string `json:"kind"`

	Match string `json:"match,omitempty"`

	ReattributedFrom string `json:"reattributed_from,omitempty"`
	TraceEventID     string `json:"trace_event_id,omitempty"`
	OccurredAt       string `json:"occurred_at,omitempty"`
	ArtifactPath     string `json:"artifact_path,omitempty"`
	ByteRange        *Range `json:"byte_range,omitempty"`
	CharRange        *Range `json:"char_range,omitempty"`
	Excerpt          string `json:"excerpt"`
	ExcerptTruncated bool   `json:"excerpt_truncated"`
	Available        bool   `json:"available"`
}

const (
	KindTraceEvent  = "trace_event"
	KindArtifact    = "artifact"
	KindAgentOutput = "agent_output"
)

const (
	MatchExact      = "exact"
	MatchNormalized = "normalized"
	MatchNotFound   = "not_found"
	MatchNotChecked = "not_checked"
)

const ExcerptLimit = 1000 // one-number: excerptLimit - what a stored EvidenceRef keeps of its source

func Excerpt(source string) (excerpt string, truncated bool) {
	runes := []rune(source)
	if len(runes) <= ExcerptLimit {
		return source, false
	}
	return string(runes[:ExcerptLimit]), true
}

func VerifiedQuote(match string) bool {
	return match == MatchExact || match == MatchNormalized
}

type CriterionResult struct {
	CriterionID string        `json:"criterion_id"`
	Text        string        `json:"text"`
	Result      string        `json:"result"`
	Source      string        `json:"source"`
	Evidence    []EvidenceRef `json:"evidence"`
	Reason      string        `json:"reason"`
	Steps       []CheckStep   `json:"steps,omitempty"`
}

type CheckStep struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Result string `json:"result"`
	Reason string `json:"reason"`
}

type Finding struct {
	Category string        `json:"category"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	Evidence []EvidenceRef `json:"evidence"`
}

func NewFinding(category, severity, message string, evidence ...EvidenceRef) Finding {
	if evidence == nil {
		evidence = []EvidenceRef{}
	}
	return Finding{Category: category, Severity: severity, Message: message, Evidence: evidence}
}

const (
	CategorySpec          = "spec"
	CategoryActivation    = "activation"
	CategoryExecution     = "execution"
	CategoryEffect        = "effect"
	CategoryCompatibility = "compatibility"
	CategoryCost          = "cost"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)
