package operations

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type FindingStatus string

const (
	FindingOpen         FindingStatus = "open"
	FindingAcknowledged FindingStatus = "acknowledged"
	FindingResolved     FindingStatus = "resolved"
	FindingDismissed    FindingStatus = "dismissed"
	FindingRecovered    FindingStatus = "recovered"
)

type FindingEventKind string

const (
	EventOpened       FindingEventKind = "opened"
	EventSeen         FindingEventKind = "seen"
	EventReopened     FindingEventKind = "reopened"
	EventRecovered    FindingEventKind = "recovered"
	EventAcknowledged FindingEventKind = "acknowledged"
	EventResolved     FindingEventKind = "resolved"
	EventDismissed    FindingEventKind = "dismissed"
)

var eventForStatus = map[FindingStatus]FindingEventKind{
	FindingOpen:         EventReopened,
	FindingRecovered:    EventRecovered,
	FindingAcknowledged: EventAcknowledged,
	FindingResolved:     EventResolved,
	FindingDismissed:    EventDismissed,
}

const findingMatchWindow = 30 * 24 * time.Hour

var (
	ErrUnknownFinding    = errors.New("operations: no finding has that id")
	ErrFindingTransition = errors.New("operations: the finding cannot move to that status from where it is")
)

type Sighting struct {
	Title    string
	Cites    []string
	Evidence map[string]any
}

type matchable struct {
	id       pgtype.UUID
	status   FindingStatus
	cites    []string
	lastSeen time.Time
}

func (f matchable) live() bool {
	return f.status == FindingOpen || f.status == FindingAcknowledged
}

var liveStatuses = []string{string(FindingOpen), string(FindingAcknowledged)}

// matchSightings pairs each sighting with at most one earlier finding that
// shares a cite, preferring live findings and then the most recently seen;
// -1 means the sighting is new.
func matchSightings(sightings []Sighting, findings []matchable) []int {
	order := make([]int, len(findings))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if findings[a].live() != findings[b].live() {
			if findings[a].live() {
				return -1
			}
			return 1
		}
		return findings[b].lastSeen.Compare(findings[a].lastSeen)
	})
	used := make([]bool, len(findings))
	matched := make([]int, len(sightings))
	for i, s := range sightings {
		matched[i] = -1
		for _, j := range order {
			if !used[j] && sharesCite(s.Cites, findings[j].cites) {
				matched[i], used[j] = j, true
				break
			}
		}
	}
	return matched
}

func sharesCite(a, b []string) bool {
	return slices.ContainsFunc(a, func(cite string) bool { return slices.Contains(b, cite) })
}

type findingRecorder struct {
	q     *gen.Queries
	tx    pgx.Tx
	run   pgtype.UUID
	agent pgtype.UUID
}

func (s *Service) recordFindings(ctx context.Context, tx pgx.Tx, run pgtype.UUID, sightings []Sighting, now time.Time) error {
	q := gen.New(tx)
	agentID, err := q.GetPlatformAgentRunAgent(ctx, run)
	if err != nil {
		return err
	}
	rows, err := q.ListMatchableFindings(ctx, gen.ListMatchableFindingsParams{
		AgentID: agentID, Live: liveStatuses, Since: pgconv.Timestamptz(now.Add(-findingMatchWindow)),
	})
	if err != nil {
		return err
	}
	findings := make([]matchable, len(rows))
	for i, row := range rows {
		findings[i] = matchable{id: row.ID, status: FindingStatus(row.Status), cites: row.Cites, lastSeen: row.LastSeenAt.Time}
	}
	r := findingRecorder{q: q, tx: tx, run: run, agent: agentID}
	matched := matchSightings(sightings, findings)
	seen := make([]bool, len(findings))
	for i, sighting := range sightings {
		j := matched[i]
		if j < 0 {
			if err := r.open(ctx, sighting); err != nil {
				return err
			}
			continue
		}
		seen[j] = true
		if err := r.see(ctx, findings[j], sighting); err != nil {
			return err
		}
	}
	for j, f := range findings {
		if !seen[j] && f.live() {
			if err := r.transition(ctx, f, FindingRecovered, nil, audit.ActionFindingRecover); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r findingRecorder) open(ctx context.Context, s Sighting) error {
	id, err := r.q.OpenFinding(ctx, gen.OpenFindingParams{AgentID: r.agent, Title: s.Title, Cites: s.Cites})
	if err != nil {
		return err
	}
	if err := r.event(ctx, id, EventOpened, &s); err != nil {
		return err
	}
	return r.audit(ctx, id, audit.ActionFindingOpen)
}

func (r findingRecorder) see(ctx context.Context, f matchable, s Sighting) error {
	if err := r.q.SeeFinding(ctx, gen.SeeFindingParams{ID: f.id, Title: s.Title, Cites: s.Cites}); err != nil {
		return err
	}
	if f.status != FindingResolved && f.status != FindingRecovered {
		return r.event(ctx, f.id, EventSeen, &s)
	}
	return r.transition(ctx, f, FindingOpen, &s, audit.ActionFindingReopen)
}

func (r findingRecorder) transition(ctx context.Context, f matchable, to FindingStatus, s *Sighting, action string) error {
	if _, err := r.q.SetFindingStatus(ctx, gen.SetFindingStatusParams{
		ID: f.id, Status: string(to), FromStatus: string(f.status),
	}); err != nil {
		return err
	}
	if err := r.event(ctx, f.id, eventForStatus[to], s); err != nil {
		return err
	}
	return r.audit(ctx, f.id, action)
}

func (r findingRecorder) event(ctx context.Context, finding pgtype.UUID, kind FindingEventKind, s *Sighting) error {
	params := gen.AppendFindingEventParams{FindingID: finding, Kind: string(kind), RunID: r.run}
	if s != nil {
		evidence, err := json.Marshal(s.Evidence)
		if err != nil {
			return err
		}
		params.Text, params.Evidence = &s.Title, evidence
	}
	return r.q.AppendFindingEvent(ctx, params)
}

func (r findingRecorder) audit(ctx context.Context, finding pgtype.UUID, action string) error {
	return audit.Log(ctx, r.tx, audit.Event{
		Agent: r.agent, Action: action, ResourceType: audit.ResourcePlatformAgentFinding, ResourceID: finding,
		Metadata: map[string]any{"run": pgconv.UUIDString(r.run)},
	})
}

var operatorTransitions = map[FindingStatus]map[FindingStatus]string{
	FindingOpen: {
		FindingAcknowledged: audit.ActionFindingAcknowledge,
		FindingResolved:     audit.ActionFindingResolve,
		FindingDismissed:    audit.ActionFindingDismiss,
	},
	FindingAcknowledged: {
		FindingResolved:  audit.ActionFindingResolve,
		FindingDismissed: audit.ActionFindingDismiss,
	},
	FindingResolved:  {FindingOpen: audit.ActionFindingReopen},
	FindingDismissed: {FindingOpen: audit.ActionFindingReopen},
	FindingRecovered: {
		FindingOpen:     audit.ActionFindingReopen,
		FindingResolved: audit.ActionFindingResolve,
	},
}

func (s *Service) MoveFinding(ctx context.Context, id pgtype.UUID, to FindingStatus, operator pgtype.UUID, note string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		current, err := q.GetFindingStatus(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownFinding
		}
		if err != nil {
			return err
		}
		from := FindingStatus(current)
		action, allowed := operatorTransitions[from][to]
		if !allowed {
			return ErrFindingTransition
		}
		params := gen.SetFindingStatusParams{ID: id, Status: string(to), FromStatus: string(from)}
		if to == FindingAcknowledged {
			params.AssigneeID = operator
		}
		if _, err := q.SetFindingStatus(ctx, params); errors.Is(err, pgx.ErrNoRows) {
			return ErrFindingTransition
		} else if err != nil {
			return err
		}
		if err := q.AppendFindingEvent(ctx, gen.AppendFindingEventParams{
			FindingID: id, Kind: string(eventForStatus[to]), OperatorID: operator, Note: &note,
		}); err != nil {
			return err
		}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: action, ResourceType: audit.ResourcePlatformAgentFinding, ResourceID: id,
			Metadata: map[string]any{"from": string(from), auditNote: note},
		})
	})
}
