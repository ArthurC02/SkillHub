package envx

import (
	"context"
	"sort"
	"strings"
	"time"
)

type Readiness string

const (
	Ready Readiness = "ready"

	Unmeasured Readiness = "unmeasured"

	Unavailable Readiness = "unavailable"

	Broken Readiness = "broken"
)

type Capability struct {
	ID string

	Name string

	Needs []string

	Without string

	Fix string

	Probe func(context.Context) error `json:"-"`
}

type Status struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Readiness   Readiness `json:"readiness"`
	Missing     []string  `json:"missing,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	Without     string    `json:"without,omitempty"`
	Fix         string    `json:"fix,omitempty"`
	MeasuredFor string    `json:"measured_for,omitempty"`
}

type Registry struct {
	caps []Capability
}

func NewRegistry(caps []Capability) *Registry {
	out := make([]Capability, len(caps))
	copy(out, caps)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return &Registry{caps: out}
}

func (r *Registry) Capabilities() []Capability { return r.caps }

func (r *Registry) DeclaredVars() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range r.caps {
		for _, n := range c.Needs {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Report(ctx context.Context, lookup func(string) string) []Status {
	out := make([]Status, 0, len(r.caps))
	for _, c := range r.caps {
		s := Status{ID: c.ID, Name: c.Name, Without: c.Without, Fix: c.Fix}
		for _, n := range c.Needs {
			if strings.TrimSpace(lookup(n)) == "" {
				s.Missing = append(s.Missing, n)
			}
		}
		switch {
		case len(s.Missing) > 0:
			s.Readiness = Unavailable
		case c.Probe == nil:

			s.Readiness = Unmeasured
			s.Detail = "前提都在，但這個部署沒有辦法量它——「設定齊全」不等於「它會動」。"
		default:
			started := time.Now()
			if err := c.Probe(ctx); err != nil {
				s.Readiness = Broken
				s.Detail = err.Error()
			} else {
				s.Readiness = Ready
			}
			s.MeasuredFor = time.Since(started).Round(time.Millisecond).String()
		}
		out = append(out, s)
	}
	return out
}

func AllReady(rows []Status) bool {
	for _, s := range rows {
		if s.Readiness != Ready {
			return false
		}
	}
	return len(rows) > 0
}
