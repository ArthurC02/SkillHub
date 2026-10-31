package main

import (
	"strings"
	"testing"
)

const wiringFixtureA = `package a

type Reader interface{ Read() string }

type Service struct {
	Read   func() string
	Reader Reader
	Name   string
}

type Params struct {
	ID    string
	Value any
}

func New(r Reader) *Service          { return &Service{Reader: r} }
func NewAll(rs ...Reader) *Service   { return &Service{} }
func Wrap(fn func() string) *Service { return &Service{Read: fn} }
func (s *Service) Own() string       { return "" }
`

const wiringFixtureB = `package b

type Service struct{ Limit string }

func (s *Service) Read() string { return "" }

func Read() string { return "" }

type Row struct{ ID string }

const Kind = "b"
`

func wiringFixtureEdges(t *testing.T, body string) map[contextEdge]string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "go.mod", "module example.com/m\n\ngo 1.27\n")
	writeAt(t, root, "internal/a/a.go", wiringFixtureA)
	writeAt(t, root, "internal/b/b.go", wiringFixtureB)
	writeAt(t, root, "internal/entrypoint/wiring/wiring.go", `package wiring

import (
	"example.com/m/internal/a"
	"example.com/m/internal/b"
)

var (
	_ = b.Kind
	_ a.Params
)

`+body)
	contextOf := func(path string) (string, bool) {
		switch path {
		case "example.com/m/internal/a":
			return "a", true
		case "example.com/m/internal/b":
			return "b", true
		}
		return "", false
	}
	edges, err := wiringEdges(root, []string{"./internal/entrypoint/..."}, contextOf)
	if err != nil {
		t.Fatalf("wiringEdges: %v", err)
	}
	return edges
}

func TestBehaviourInjectedIntoAnotherContextIsAnEdge(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, at string
	}{
		{"a function in a struct field", "func w() { _ = a.Service{Read: b.Read} }\n", "wiring.go:13"},
		{"a method value assigned to a field", "func w(s *a.Service, bs *b.Service) { s.Read = bs.Read }\n", "wiring.go:13"},
		{"a service object in a struct field", "func w(bs *b.Service) { _ = &a.Service{Reader: bs} }\n", "wiring.go:13"},
		{"a constructor parameter the context declares", "func w(bs *b.Service) { _ = a.New(bs) }\n", "wiring.go:13"},
		{"a variadic parameter the context declares", "func w(bs *b.Service) { _ = a.NewAll(bs, bs) }\n", "wiring.go:13"},
		{"a service object held in a field", "type deps struct{ B *b.Service }\n\nfunc w(d deps) { _ = a.New(d.B) }\n", "wiring.go:15"},
		{"a closure calling the other context", "func w(bs *b.Service) {\n\t_ = a.Service{Read: func() string { return bs.Read() }}\n}\n", "wiring.go:14"},
	}
	for _, c := range cases {
		edges := wiringFixtureEdges(t, c.body)
		at, ok := edges[contextEdge{"a", "b"}]
		if !ok || len(edges) != 1 || !strings.HasSuffix(at, c.at) {
			t.Errorf("%s: %v, want only a->b at %s", c.name, edges, c.at)
		}
	}
}

func TestDataAndForeignSlotsAreNotCollaborations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body string
	}{
		{"a constant", "func w() { _ = a.Service{Name: b.Kind} }\n"},
		{"a field read off a data row", "func w(r b.Row) { _ = a.Params{ID: r.ID} }\n"},
		{"a whole data row with no methods", "func w(r b.Row) { _ = a.Params{Value: r} }\n"},
		{"a parameter the context does not declare", "func w(bs *b.Service) { _ = a.Wrap(bs.Read) }\n"},
		{"a value read through a service object", "type deps struct{ B *b.Service }\n\nfunc w(d deps) { _ = a.Params{ID: d.B.Limit} }\n"},
		{"a context wiring itself", "func w(s *a.Service) { s.Read = s.Own }\n"},
		{"a slot outside any context", "type local struct{ Read func() string }\n\nfunc w() { _ = local{Read: b.Read} }\n"},
	}
	for _, c := range cases {
		if edges := wiringFixtureEdges(t, c.body); len(edges) != 0 {
			t.Errorf("%s: %v, want none", c.name, edges)
		}
	}
}

func TestOnlyUnregisteredEdgesOutsideTheBaselineFail(t *testing.T) {
	t.Parallel()
	edges := map[contextEdge]string{
		{"eval", "run"}:     "wiring/batches.go:25",
		{"eval", "testlab"}: "wiring/trial.go:10",
		{"identity", "run"}: "app.go:208",
		{"registry", "run"}: "app.go:199",
	}
	registered := func(e contextEdge) bool { return e == contextEdge{"eval", "testlab"} }
	p := unregisteredEdgeProblems(edges, registered, []string{"identity->run", "registry->run", "trace->run"})
	want := []string{
		"eval->run (wiring/batches.go:25) wires one Context into another",
		"trace->run is registered or no longer wired",
	}
	if len(p) != len(want) {
		t.Fatalf("problems: %v, want %d", p, len(want))
	}
	for i, w := range want {
		if !strings.Contains(p[i], w) {
			t.Errorf("problem %d: %q, want it to say %q", i, p[i], w)
		}
	}
}

func TestAReviewedInteractionRegistersItsConsumerToProducerEdge(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAt(t, root, interactionsFile+".json", `{"interactions": [
		{"consumer_context": "run", "producer_context": "registry", "status": "reviewed"},
		{"consumer_context": "eval", "producer_context": "run", "status": "candidate"}
	]}`)
	got, err := reviewedInteractions(root)
	if err != nil {
		t.Fatal(err)
	}
	if !got[contextEdge{"run", "registry"}] || got[contextEdge{"registry", "run"}] || got[contextEdge{"eval", "run"}] {
		t.Errorf("interactions: %v, want only the reviewed run->registry", got)
	}
}

func TestTheCompositionRootWiresOnlyRegisteredCollaborations(t *testing.T) {
	t.Parallel()
	for _, problem := range wiringInteractionProblems("../..") {
		t.Error(problem)
	}
}
