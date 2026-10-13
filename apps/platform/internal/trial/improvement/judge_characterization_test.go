package eval

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

func TestACitedTraceEventWithoutAQuoteResolvesButIsNotACheckedQuote(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID)}, m, digest)
	if why != "" {
		t.Fatalf("a cited event that is in the digest resolves: %q", why)
	}
	if got.Match != verdict.MatchNotChecked || got.TraceEventID != eventID || got.ReattributedFrom != "" {
		t.Errorf("got %+v, want an unreattributed reference to %s marked not_checked: no quote was compared", got, eventID)
	}
}

func TestAQuoteFoundInTheCitedEventIsNotReattributed(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{Kind: verdict.KindTraceEvent, TraceEventID: strp(eventID), Quote: `"tool_name":"bash"`}, m, digest)
	if why != "" {
		t.Fatalf("a quote inside the cited event resolves: %q", why)
	}
	if got.Kind != verdict.KindTraceEvent || got.TraceEventID != eventID || got.ReattributedFrom != "" {
		t.Errorf("got %+v, want the cited event itself with no reattribution", got)
	}
}

func TestAnAgentOutputQuoteThatIsOnlyInTheTraceIsReattributedToTheTrace(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{Kind: verdict.KindAgentOutput, Quote: `"tool_name":"bash"`}, m, digest)
	if why != "" {
		t.Fatalf("the quote is in this run's trace, so the citation holds: %q", why)
	}
	if got.Kind != verdict.KindTraceEvent || got.ReattributedFrom != verdict.KindAgentOutput {
		t.Errorf("got kind %q reattributed from %q, want %q from %q", got.Kind, got.ReattributedFrom, verdict.KindTraceEvent, verdict.KindAgentOutput)
	}
}

func TestATraceEventOutsideTheDigestStillResolvesWhenItsQuoteIsElsewhere(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{
		Kind: verdict.KindTraceEvent, TraceEventID: strp("11111111-1111-4111-8111-111111111111"), Quote: "Removed 17 duplicate rows",
	}, m, digest)
	if why != "" {
		t.Fatalf("the quote is in the final output, so the citation holds: %q", why)
	}
	if got.Kind != verdict.KindAgentOutput || got.ReattributedFrom != verdict.KindTraceEvent {
		t.Errorf("got kind %q reattributed from %q, want %q from %q", got.Kind, got.ReattributedFrom, verdict.KindAgentOutput, verdict.KindTraceEvent)
	}
}

func TestAReattributedQuoteThatOnlyMatchesAfterNormalisationSaysSo(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(modelport.Citation{
		Kind: verdict.KindArtifact, ArtifactPath: strp("output.xlsx"), Quote: "Removed 17  duplicate rows and saved}],",
	}, m, digest)
	if why != "" {
		t.Fatalf("the normalised quote is in the final output: %q", why)
	}
	if got.Match != verdict.MatchNormalized || got.ReattributedFrom != verdict.KindArtifact {
		t.Errorf("match %q reattributed from %q, want %q from %q", got.Match, got.ReattributedFrom, verdict.MatchNormalized, verdict.KindArtifact)
	}
}
