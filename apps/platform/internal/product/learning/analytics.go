package analytics

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"unicode"

	"crypto/rand"
	"encoding/hex"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	EventSearchPerformed   = "search_performed"
	EventSkillDetailViewed = "skill_detail_viewed"

	EventSessionStarted = "session_started"

	EventDownloadStarted = "download_started"
)

const (
	sessionCookie = "sh_analytics"
	visitCookie   = "sh_analytics_visit"
)

type Service struct {
	Pool *pgxpool.Pool

	RunBelongsToWorkspace func(context.Context, pgtype.UUID, pgtype.UUID) (bool, error)

	Retention time.Duration

	Secure bool
	Now    func() time.Time
}

func (s *Service) Enabled() bool {
	return s != nil && s.Pool != nil && s.Retention >= time.Second
}

type ctxKey struct{}

func SessionID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func (s *Service) Sessions(next http.Handler) http.Handler {
	if !s.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A freshly minted id is offered via Set-Cookie but not attached to
		// this request's context: several requests can arrive cold at once,
		// each would mint its own id, and only the one the browser echoes back is real.
		c, err := r.Cookie(sessionCookie)
		if err != nil || len(c.Value) != 32 {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {

				next.ServeHTTP(w, r)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name: sessionCookie, Value: hex.EncodeToString(raw), Path: "/",
				MaxAge:   int(s.Retention / time.Second),
				HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode,
			})

			next.ServeHTTP(w, r)
			return
		}
		id := c.Value
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

func (s *Service) SkillDetailViewed(ctx context.Context, workspace, skillID pgtype.UUID) {
	if !s.Enabled() {
		return
	}
	s.emit(ctx, gen.InsertAnalyticsEventParams{
		EventName:   EventSkillDetailViewed,
		SessionID:   SessionID(ctx),
		WorkspaceID: workspace,
		SkillID:     skillID,
	})
}

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

func (s *Service) emit(ctx context.Context, p gen.InsertAnalyticsEventParams) {
	if p.SessionID == "" {

		return
	}
	if err := gen.New(s.Pool).InsertAnalyticsEvent(ctx, p); err != nil {
		slog.Warn("analytics event not recorded", "event", p.EventName, "error", err)
	}
}

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
