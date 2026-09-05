package creation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// Diagram interpretations have four explicit sections, even when a section is
// empty. A paragraph cannot silently omit an uncertain branch before confirmation.
func validDiagramInterpretation(value string) bool {
	var sections map[string][]string
	if json.Unmarshal([]byte(value), &sections) != nil || len(sections) != 4 || len(sections["nodes"]) == 0 {
		return false
	}
	for name, limit := range map[string]int{"nodes": 64, "conditions": 64, "branches": 128, "uncertainties": 64} {
		items, ok := sections[name]
		if !ok || items == nil || len(items) > limit {
			return false
		}
		for _, item := range items {
			if strings.TrimSpace(item) == "" || utf8.RuneCountInString(item) > 2000 {
				return false
			}
		}
	}
	return utf8.RuneCountInString(value) <= 20000
}

type TransientRequest struct {
	WorkspaceID      pgtype.UUID               `json:"workspace_id"`
	SessionID        pgtype.UUID               `json:"session_id"`
	ReceiptID        pgtype.UUID               `json:"receipt_id"`
	ExpectedRevision int64                     `json:"expected_revision"`
	Diagram          llmclient.GenerateDiagram `json:"diagram"`
}

func diagramMatches(p Snapshot, d *llmclient.GenerateDiagram) bool {
	if d == nil {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(d.Data)
	if err != nil || len(b) == 0 || len(b) > MaxDiagramBytes {
		return false
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) == p.DiagramFingerprint && d.MediaType == p.DiagramMediaType && len(b) == p.DiagramBytes
}

// TransientHandler exists only on the Go Worker's internal service listener.
func (s *Service) TransientHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := sha256.Sum256([]byte("Bearer " + token))
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if token == "" || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/v1/creation/transient-step" {
			http.NotFound(w, r)
			return
		}
		var in TransientRequest
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		a := JobArgs{in.SessionID, in.WorkspaceID, in.ExpectedRevision, in.ReceiptID}
		if err := s.Step(r.Context(), a, &in.Diagram); err != nil {
			switch {
			case errors.Is(err, ErrConflict):
				http.Error(w, "consumed or stale command", http.StatusConflict)
			case errors.Is(err, ErrInvalidCommand):
				http.Error(w, "invalid transient input", http.StatusBadRequest)
			default:
				http.Error(w, "step unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// TransientClient sends image bytes only in the current HTTP exchange.
func TransientClient(baseURL, token string, timeout time.Duration) func(context.Context, JobArgs, *llmclient.GenerateDiagram) error {
	if baseURL == "" || token == "" || timeout <= 0 {
		return nil
	}
	return func(ctx context.Context, a JobArgs, d *llmclient.GenerateDiagram) error {
		if d == nil {
			return ErrInvalidCommand
		}
		b, err := json.Marshal(TransientRequest{a.WorkspaceID, a.SessionID, a.ReceiptID, a.Revision, *d})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/creation/transient-step", bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return ErrUnavailable
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
		if res.StatusCode != 200 {
			return ErrUnavailable
		}
		return nil
	}
}

// InterruptedTransient records the lost upload without replaying its bytes.
func (s *Service) InterruptedTransient(ctx context.Context, a JobArgs) error {
	return s.recoverAttempt(ctx, a, true)
}
func (s *Service) recoverAttempt(ctx context.Context, a JobArgs, force bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	row, err := q.LockCreationSession(ctx, gen.LockCreationSessionParams{ID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	e, err := decode(row)
	if err != nil {
		return err
	}
	if e.ActiveReceipt != a.ReceiptID || (row.State != "queued" && row.State != "working") {
		return nil
	}
	if !force && row.State == "working" && e.ActiveDeadline.After(time.Now()) {
		return nil
	}
	receipt, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	status := "failed"
	if receipt.Status == "running" {
		e.Snapshot.UsageUnknown = true
		status = "unknown"
	}
	state := "failed"
	if e.Snapshot.DiagramFingerprint != "" && e.Snapshot.DiagramUnderstanding == "" {
		state = "needs_reupload"
	}
	e.ActiveReceipt = pgtype.UUID{}
	e.Snapshot.PendingAction = ""
	e.Snapshot.Messages = append(e.Snapshot.Messages, llmclient.CreationMessage{Role: "assistant", Content: "工作已中斷，已保留進度。費用無法確認時仍占用預算；流程圖請重新上傳。"})
	if _, err = advance(ctx, tx, row, state, "attempt_interrupted", e); err != nil {
		return err
	}
	_, err = q.FinishCreationReceipt(ctx, gen.FinishCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID, Status: status, Result: []byte("{}"), Usage: []byte("null")})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) Recover(ctx context.Context) error {
	if !s.Limits.Valid() {
		return ErrUnavailable
	}
	rows, err := gen.New(s.Pool).ListStalledCreationSessions(ctx, pgtype.Timestamptz{Time: time.Now().Add(-s.Limits.CallTimeout - 15*time.Second), Valid: true})
	if err != nil {
		return err
	}
	for _, row := range rows {
		e, err := decode(row)
		if err != nil {
			return err
		}
		// A queued row is a River job still healthy in the queue (a worker
		// restart or a backlog, not a stalled attempt); only fail it once the
		// session's own wall clock has actually passed. A working row keeps
		// the existing call-timeout threshold.
		if row.State == "queued" && e.Deadline.After(time.Now()) {
			continue
		}
		if err = s.recoverAttempt(ctx, JobArgs{row.ID, row.WorkspaceID, row.Revision, e.ActiveReceipt}, false); err != nil {
			return err
		}
		if s.RevokeKey != nil && e.ActiveReceipt.Valid {
			_ = s.RevokeKey(ctx, UUID(e.ActiveReceipt))
		}
	}
	return nil
}
