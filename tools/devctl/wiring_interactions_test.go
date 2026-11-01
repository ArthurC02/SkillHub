package main

import (
	"fmt"
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

type wiringCase struct {
	name, body, at string
}

func (c wiringCase) contexts(i int) (string, string) {
	return fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i)
}

func wiringFixtureEdges(t *testing.T, cases []wiringCase) map[contextEdge]string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "go.mod", "module example.com/m\n\ngo 1.27\n")
	contexts := map[string]string{}
	for i, c := range cases {
		a, b := c.contexts(i)
		contexts["example.com/m/internal/"+a] = a
		contexts["example.com/m/internal/"+b] = b
		writeAt(t, root, "internal/"+a+"/a.go", wiringFixtureA)
		writeAt(t, root, "internal/"+b+"/b.go", wiringFixtureB)
		writeAt(t, root, fmt.Sprintf("internal/entrypoint/w%d/wiring.go", i), `package wiring

import (
	a "example.com/m/internal/`+a+`"
	b "example.com/m/internal/`+b+`"
)

var (
	_ = b.Kind
	_ a.Params
)

`+c.body)
	}
	contextOf := func(path string) (string, bool) {
		name, ok := contexts[path]
		return name, ok
	}
	edges, err := wiringEdges(root, []string{"./internal/entrypoint/..."}, contextOf)
	if err != nil {
		t.Fatalf("wiringEdges: %v", err)
	}
	return edges
}

func TestBehaviourInjectedIntoAnotherContextIsAnEdge(t *testing.T) {
	t.Parallel()
	cases := []wiringCase{
		{"a function in a struct field", "func w() { _ = a.Service{Read: b.Read} }\n", "wiring.go:13"},
		{"a method value assigned to a field", "func w(s *a.Service, bs *b.Service) { s.Read = bs.Read }\n", "wiring.go:13"},
		{"a service object in a struct field", "func w(bs *b.Service) { _ = &a.Service{Reader: bs} }\n", "wiring.go:13"},
		{"a constructor parameter the context declares", "func w(bs *b.Service) { _ = a.New(bs) }\n", "wiring.go:13"},
		{"a variadic parameter the context declares", "func w(bs *b.Service) { _ = a.NewAll(bs, bs) }\n", "wiring.go:13"},
		{"a service object held in a field", "type deps struct{ B *b.Service }\n\nfunc w(d deps) { _ = a.New(d.B) }\n", "wiring.go:15"},
		{"a closure calling the other context", "func w(bs *b.Service) {\n\t_ = a.Service{Read: func() string { return bs.Read() }}\n}\n", "wiring.go:14"},
	}
	edges := wiringFixtureEdges(t, cases)
	if len(edges) != len(cases) {
		t.Errorf("edges: %v, want exactly one per case", edges)
	}
	for i, c := range cases {
		a, b := c.contexts(i)
		at, ok := edges[contextEdge{a, b}]
		if !ok || !strings.HasSuffix(at, fmt.Sprintf("w%d/%s", i, c.at)) {
			t.Errorf("%s: %v, want %s->%s at %s", c.name, edges, a, b, c.at)
		}
	}
}

func TestDataAndForeignSlotsAreNotCollaborations(t *testing.T) {
	t.Parallel()
	cases := []wiringCase{
		{"a constant", "func w() { _ = a.Service{Name: b.Kind} }\n", ""},
		{"a field read off a data row", "func w(r b.Row) { _ = a.Params{ID: r.ID} }\n", ""},
		{"a whole data row with no methods", "func w(r b.Row) { _ = a.Params{Value: r} }\n", ""},
		{"a parameter the context does not declare", "func w(bs *b.Service) { _ = a.Wrap(bs.Read) }\n", ""},
		{"a value read through a service object", "type deps struct{ B *b.Service }\n\nfunc w(d deps) { _ = a.Params{ID: d.B.Limit} }\n", ""},
		{"a context wiring itself", "func w(s *a.Service) { s.Read = s.Own }\n", ""},
		{"a slot outside any context", "type local struct{ Read func() string }\n\nfunc w() { _ = local{Read: b.Read} }\n", ""},
	}
	if edges := wiringFixtureEdges(t, cases); len(edges) != 0 {
		t.Errorf("%v, want none", edges)
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
