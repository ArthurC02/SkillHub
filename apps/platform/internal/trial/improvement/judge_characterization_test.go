package eval

import "testing"

func TestACitedTraceEventWithoutAQuoteIsAnExactMatchOnThatEvent(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(Citation{Kind: KindTraceEvent, TraceEventID: strp(eventID)}, m, digest)
	if why != "" {
		t.Fatalf("a cited event that is in the digest resolves: %q", why)
	}
	if got.Match != MatchExact || got.TraceEventID != eventID || got.ReattributedFrom != "" {
		t.Errorf("got %+v, want an exact, unreattributed reference to %s", got, eventID)
	}
}

func TestAQuoteFoundInTheCitedEventIsNotReattributed(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(Citation{Kind: KindTraceEvent, TraceEventID: strp(eventID), Quote: `"tool_name":"bash"`}, m, digest)
	if why != "" {
		t.Fatalf("a quote inside the cited event resolves: %q", why)
	}
	if got.Kind != KindTraceEvent || got.TraceEventID != eventID || got.ReattributedFrom != "" {
		t.Errorf("got %+v, want the cited event itself with no reattribution", got)
	}
}

func TestAnAgentOutputQuoteThatIsOnlyInTheTraceIsReattributedToTheTrace(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(Citation{Kind: KindAgentOutput, Quote: `"tool_name":"bash"`}, m, digest)
	if why != "" {
		t.Fatalf("the quote is in this run's trace, so the citation holds: %q", why)
	}
	if got.Kind != KindTraceEvent || got.ReattributedFrom != KindAgentOutput {
		t.Errorf("got kind %q reattributed from %q, want %q from %q", got.Kind, got.ReattributedFrom, KindTraceEvent, KindAgentOutput)
	}
}

func TestATraceEventOutsideTheDigestStillResolvesWhenItsQuoteIsElsewhere(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(Citation{
		Kind: KindTraceEvent, TraceEventID: strp("11111111-1111-4111-8111-111111111111"), Quote: "Removed 17 duplicate rows",
	}, m, digest)
	if why != "" {
		t.Fatalf("the quote is in the final output, so the citation holds: %q", why)
	}
	if got.Kind != KindAgentOutput || got.ReattributedFrom != KindTraceEvent {
		t.Errorf("got kind %q reattributed from %q, want %q from %q", got.Kind, got.ReattributedFrom, KindAgentOutput, KindTraceEvent)
	}
}

func TestAReattributedQuoteThatOnlyMatchesAfterNormalisationSaysSo(t *testing.T) {
	m, digest := fixtureMaterial(true)
	got, why := verify(Citation{
		Kind: KindArtifact, ArtifactPath: strp("output.xlsx"), Quote: "Removed 17  duplicate rows and saved}],",
	}, m, digest)
	if why != "" {
		t.Fatalf("the normalised quote is in the final output: %q", why)
	}
	if got.Match != MatchNormalized || got.ReattributedFrom != KindArtifact {
		t.Errorf("match %q reattributed from %q, want %q from %q", got.Match, got.ReattributedFrom, MatchNormalized, KindArtifact)
	}
}
