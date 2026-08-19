// Package analytics writes the product analytics events of the core funnel
// (02:O11Y-004, ADR-029) — the fifth data class, and the only one that is
// explicitly not a source of truth.
//
// What that means in practice, because it is the whole design:
//
//   - Any funnel segment an existing domain table can answer is answered there.
//     Runs come from `runs`, "was it helpful" from evaluations.feedback_helpful,
//     "was the advice taken" from evaluation_suggestions.decision, "did they
//     download" from download_records. Only four segments have no table behind
//     them, and those four are the only events this package emits. A fifth one
//     has to explain first why no domain table answers it (ADR-029 決策 1).
//   - No free text, ever. Not masked free text — none. Each event has its own
//     constructor below, and that signature is the attribute whitelist: an
//     attribute nobody declared cannot be passed, so it cannot be stored. Iron
//     rule 11's ban on secrets in "分析事件" is met by structure rather than by a
//     masking pass, because unlike a Trace there is nothing here to mask.
//   - The search query's words are not recorded. Its length and a coarse script
//     bucket are. A query can carry personal data, and the only channel that may
//     see the words is BETA-003's, which has explicit consent (ADR-029 決策 2).
//   - The session identifier is not the ADR-020 session token, nor a hash of it,
//     nor anything derivable from it. That is credential material. This is an
//     unrelated random cookie value that cannot be resolved back to a sessions
//     row, and before login it is the only linkage that exists (ADR-029 決策 4).
//
// Nothing is collected at all until a deployment sets a retention period. That is
// not caution for its own sake: NFR-002 requires a retention value to exist before
// a data class starts accumulating, ADR-029 決策 5 proposes 180 days and that
// proposal is not ratified, and this is the one data class that can still be built
// in the right order because it has not started yet.
package analytics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode"

	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// The four event names, matching the CHECK in 0029. Stable strings: they end up
// in stored rows that outlive any refactor of these constants.
const (
	// EventSearchPerformed and EventSkillDetailViewed are funnel segment 1,
	// "輸入意圖後查看至少一個詳情" — the segment that happens before any run and
	// often before any login, which is why neither Run Trace nor an audit event
	// can answer it (they are the wrong category, not merely unimplemented).
	EventSearchPerformed   = "search_performed"
	EventSkillDetailViewed = "skill_detail_viewed"
	// EventSessionStarted is funnel segment 7, "首次使用後再回來". It is also what
	// BETA-004's blocking reports are strung onto: a problem counts as blocking
	// when a user stopped at a segment and did not work around it, and answering
	// that needs the reports and the funnel to share one session identifier.
	EventSessionStarted = "session_started"
	// EventDownloadStarted is the first half of funnel segment 6. The second half
	// — the download that succeeded — stays in download_records, because that is a
	// domain fact and this is not.
	EventDownloadStarted = "download_started"
)

// sessionCookie carries the analytics session id. A separate cookie from
// sh_session on purpose (ADR-029 決策 4): it outlives a login, exists without one,
// and must never be mistakable for a credential. HttpOnly anyway — the SPA has no
// reason to read it, and a value scripts can read is a value an injected script
// can read.
const (
	sessionCookie = "sh_analytics"
	visitCookie   = "sh_analytics_visit"
)

// Service writes funnel events.
type Service struct {
	Pool *pgxpool.Pool
	// Retention is how long an event is kept, and the cookie's own lifetime.
	// Zero — the shipped default — means this deployment collects nothing: no
	// cookie is set, no row is written, and Enabled() is false everywhere.
	// ADR-029 決策 5 proposes 180 days; until that is ratified the honest state of
	// a deployment is "not collecting" (NFR-002 未定值前不得開始收集).
	Retention time.Duration
	// Secure controls the cookie Secure flag; false only for plain-http local dev,
	// same as the session cookie's.
	Secure bool
	Now    func() time.Time
}

// Enabled reports whether this deployment collects funnel events at all.
func (s *Service) Enabled() bool {
	return s != nil && s.Pool != nil && s.Retention >= time.Second
}

type ctxKey struct{}

// SessionID returns the analytics session id Sessions put in the context, or ""
// when collection is off or the middleware did not run.
func SessionID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Sessions is the middleware that gives every visitor an analytics session id and
// emits a best-effort EventSessionStarted marker once per UTC day. Concurrent
// first requests may emit duplicates; the funnel deliberately counts distinct
// session/day pairs. The persistent analytics id ties visits together without
// using an authentication credential.
//
// Wrapped around the whole mux rather than per route, because the funnel's first
// segment happens on the public catalogue where there is no session middleware at
// all — measuring only signed-in traffic would drop exactly the population the
// funnel exists to study.
//
// It is a no-op when collection is off, including the cookie: a deployment that
// has not set a retention period sets nothing on a visitor's browser either.
func (s *Service) Sessions(next http.Handler) http.Handler {
	if !s.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if c, err := r.Cookie(sessionCookie); err == nil && len(c.Value) == 32 {
			id = c.Value
		} else {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				// No id, no event, request carries on. Analytics never fails a
				// user's request (ADR-029 決策 1).
				next.ServeHTTP(w, r)
				return
			}
			id = hex.EncodeToString(raw)
			http.SetCookie(w, &http.Cookie{
				Name: sessionCookie, Value: id, Path: "/",
				MaxAge:   int(s.Retention / time.Second),
				HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode,
			})
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		now := s.now()
		if newVisit(r, now) {
			seconds := visitLifetimeSeconds(now, s.Retention)
			http.SetCookie(w, &http.Cookie{
				Name: visitCookie, Value: now.UTC().Format("2006-01-02"), Path: "/",
				MaxAge: seconds, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode,
			})
			s.emit(ctx, gen.InsertAnalyticsEventParams{
				EventName: EventSessionStarted, SessionID: id,
			})
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func newVisit(r *http.Request, now time.Time) bool {
	c, err := r.Cookie(visitCookie)
	return err != nil || c.Value != now.UTC().Format("2006-01-02")
}

func visitLifetimeSeconds(now time.Time, retention time.Duration) int {
	seconds := int(now.UTC().Truncate(24*time.Hour).Add(24*time.Hour).Sub(now.UTC()) / time.Second)
	if sessionSeconds := int(retention / time.Second); seconds > sessionSeconds {
		seconds = sessionSeconds
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

// PurgeExpired enforces the configured analytics retention window.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	if s == nil || s.Pool == nil {
		return 0, errors.New("analytics purge requires a database pool")
	}
	if s.Retention <= 0 {
		return 0, errors.New("analytics purge requires a positive retention period")
	}
	return gen.New(s.Pool).DeleteExpiredAnalyticsEvents(ctx,
		pgtype.Timestamptz{Time: s.now().Add(-s.Retention), Valid: true})
}

// SearchPerformed records that an intent was submitted (funnel segment 1).
//
// The parameter list is the whitelist: length and script of the query, whether it
// found anything, how much, and whether filters were on. The query itself is not a
// parameter, so no caller can pass it by accident.
func (s *Service) SearchPerformed(ctx context.Context, query string, results int, filtersApplied bool) {
	if !s.Enabled() {
		return
	}
	length := int32(len([]rune(query)))
	language := queryScript(query)
	count := int32(results)
	has := results > 0
	s.emit(ctx, gen.InsertAnalyticsEventParams{
		EventName:      EventSearchPerformed,
		SessionID:      SessionID(ctx),
		QueryLength:    &length,
		QueryLanguage:  &language,
		ResultCount:    &count,
		HasResults:     &has,
		FiltersApplied: &filtersApplied,
	})
}

// SkillDetailViewed records that a detail page was opened (funnel segment 1's
// other half). arrival says how they got there and rank is their position in the
// result list when they came from search; both come from the client, and both are
// clamped to the whitelist here rather than trusted.
func (s *Service) SkillDetailViewed(
	ctx context.Context, workspace, skillID pgtype.UUID, arrival string, rank int,
) {
	if !s.Enabled() {
		return
	}
	if arrival != "search" {
		arrival = "direct"
	}
	p := gen.InsertAnalyticsEventParams{
		EventName:   EventSkillDetailViewed,
		SessionID:   SessionID(ctx),
		WorkspaceID: workspace,
		SkillID:     skillID,
		Arrival:     &arrival,
	}
	if arrival == "search" && rank >= 1 {
		r := int32(rank)
		p.ArrivalRank = &r
	}
	s.emit(ctx, p)
}

// DownloadStarted records that a download was requested (funnel segment 6's first
// half). The half that succeeded is download_records and stays there — this event
// exists precisely so the two can be told apart.
func (s *Service) DownloadStarted(ctx context.Context, workspace, artifactID pgtype.UUID, target string) {
	if !s.Enabled() {
		return
	}
	p := gen.InsertAnalyticsEventParams{
		EventName:   EventDownloadStarted,
		SessionID:   SessionID(ctx),
		WorkspaceID: workspace,
		ArtifactID:  artifactID,
	}
	if target != "" {
		p.Target = &target
	}
	s.emit(ctx, p)
}

// emit writes one row, best effort.
//
// A failure is logged and swallowed. Analytics is not a source of truth (ADR-029
// 決策 1), so a lost row costs a data point; failing a user's search because a
// measurement failed would invert that priority exactly.
//
// ponytail: synchronous, one INSERT on the request path. The ceiling is the write
// latency added to a search at beta scale, which is a dozen users. Upgrade path if
// that ever shows up in the NFR-004 percentile: a buffered channel drained by one
// goroutine — not a queue, because a dropped analytics event is allowed to be
// dropped and a queue would be machinery defending something that does not need
// defending.
func (s *Service) emit(ctx context.Context, p gen.InsertAnalyticsEventParams) {
	if p.SessionID == "" {
		// No session id means the middleware did not run for this request. Writing
		// the row anyway would produce a funnel step belonging to nobody.
		return
	}
	if err := gen.New(s.Pool).InsertAnalyticsEvent(ctx, p); err != nil {
		slog.Warn("analytics event not recorded", "event", p.EventName, "error", err)
	}
}

// queryScript buckets a query by writing system, which is as close to "language"
// as this package is willing to get: a coarse bucket cannot reconstruct anything,
// and the question the funnel actually asks is whether cross-script retrieval
// (ADR-013's vector leg) is carrying non-English intents.
func queryScript(q string) string {
	han, latin := false, false
	for _, r := range q {
		switch {
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r),
			unicode.Is(unicode.Hangul, r):
			han = true
		case unicode.Is(unicode.Latin, r):
			latin = true
		}
	}
	switch {
	case han && latin:
		return "mixed"
	case han:
		return "han"
	case latin:
		return "latin"
	default:
		return "other"
	}
}

// ArrivalFromRequest reads how a visitor reached a detail page. Query parameters
// because only the client knows, and clamped on the way in: `from` has to be one
// of two words and `rank` has to be a small positive integer, so a crafted link
// cannot write anything else into the table.
func ArrivalFromRequest(r *http.Request) (string, int) {
	arrival := r.URL.Query().Get("from")
	if arrival != "search" {
		return "direct", 0
	}
	rank, err := strconv.Atoi(r.URL.Query().Get("rank"))
	if err != nil || rank < 1 || rank > 1000 {
		rank = 0
	}
	return "search", rank
}
