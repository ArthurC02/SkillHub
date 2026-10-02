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
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type TransientRequest struct {
	WorkspaceID      pgtype.UUID `json:"workspace_id"`
	SessionID        pgtype.UUID `json:"session_id"`
	ReceiptID        pgtype.UUID `json:"receipt_id"`
	ExpectedRevision int64       `json:"expected_revision"`
	Diagram          Diagram     `json:"diagram"`
}

func diagramMatches(p Snapshot, d *Diagram) bool {
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

const maxTransientRequestBytes = 8 << 20

func (s *Service) TransientHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := sha256.Sum256([]byte("Bearer " + token))
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if token == "" || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/creation/transient-step" {
			http.NotFound(w, r)
			return
		}
		var in TransientRequest
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTransientRequestBytes))
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

func TransientClient(baseURL, token string, timeout time.Duration) func(context.Context, JobArgs, *Diagram) error {
	return TransientClientWithHTTP(baseURL, token, timeout, nil)
}

func TransientClientWithHTTP(baseURL, token string, timeout time.Duration, client *http.Client) func(context.Context, JobArgs, *Diagram) error {
	if baseURL == "" || token == "" || timeout <= 0 {
		return nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	return func(ctx context.Context, a JobArgs, d *Diagram) error {
		if d == nil {
			return ErrInvalidCommand
		}
		b, err := json.Marshal(TransientRequest{a.WorkspaceID, a.SessionID, a.ReceiptID, a.Revision, *d})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/creation/transient-step", bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%w: the worker answered %s", ErrUnavailable, res.Status)
		}
		return nil
	}
}

const interruptTimeout = 10 * time.Second

func (s *Service) HandOffStep(ctx context.Context, a JobArgs, d *Diagram) {
	err := ErrUnavailable
	if s.HandOff != nil {
		err = s.HandOff(ctx, a, d)
	}
	if err == nil {
		return
	}
	slog.Warn("creation: a step could not be handed off and is interrupted", "session_id", UUID(a.SessionID), "error", err)
	interruptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interruptTimeout)
	defer cancel()
	if interruptErr := s.InterruptedTransient(interruptCtx, a); interruptErr != nil {
		slog.Error("creation: a step that could not be handed off was not interrupted; the stalled-session sweep will end it",
			"session_id", UUID(a.SessionID), "hand_off_error", err, "error", interruptErr)
	}
}

func (s *Service) InterruptedTransient(ctx context.Context, a JobArgs) error {
	return s.recoverAttempt(ctx, a, nothingSpared)
}

func nothingSpared(State, envelope) bool { return false }

func stillWithinCallDeadline(state State, e envelope) bool {
	return state == StateWorking && e.ActiveDeadline.After(time.Now())
}

func (s *Service) recoverAttempt(ctx context.Context, a JobArgs, spared func(State, envelope) bool) error {
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
	if e.ActiveReceipt != a.ReceiptID || !State(row.State).AwaitsTheModel() {
		return nil
	}
	if spared(State(row.State), e) {
		return nil
	}
	receipt, err := q.GetCreationReceipt(ctx, gen.GetCreationReceiptParams{ID: a.ReceiptID, SessionID: a.SessionID, WorkspaceID: a.WorkspaceID})
	if err != nil {
		return err
	}
	status := receiptFailed
	if receipt.Status == receiptRunning {
		e.Snapshot.UsageUnknown = true
		status = receiptUnknown
	}
	state := abandonedState(e.Snapshot)
	e.ActiveReceipt = pgtype.UUID{}
	e.Snapshot.PendingAction = NothingPending
	e.Snapshot.appendMessage("assistant", "工作已中斷，已保留進度。費用無法確認的那一步不會向你收費，但仍占用這次的預算額度；流程圖請重新上傳。")
	if _, err = s.advance(ctx, tx, row, transition{state, "attempt_interrupted"}, e); err != nil {
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
	rows, err := gen.New(s.Pool).ListStalledCreationSessions(ctx, gen.ListStalledCreationSessionsParams{
		States:        statesAwaitingTheModel(),
		StalledBefore: pgtype.Timestamptz{Time: time.Now().Add(-s.Limits.CallTimeout - 15*time.Second), Valid: true},
		BatchSize:     stalledSessionBatch,
	})
	if err != nil {
		return err
	}
	var failed []error
	for _, row := range rows {
		if err := s.recoverStalled(ctx, row); err != nil {
			slog.Warn("creation: a stalled session was not recovered; the rest of the batch still is",
				"session_id", UUID(row.ID), "error", err)
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

func (s *Service) recoverStalled(ctx context.Context, row gen.CreationSession) error {
	e, err := decode(row)
	if err != nil {
		return err
	}
	// A queued row can just be a healthy backlog, not a stalled attempt;
	// only fail it once its own deadline has passed.
	if State(row.State) == StateQueued && e.Deadline.After(time.Now()) {
		return nil
	}
	if err = s.recoverAttempt(ctx, JobArgs{row.ID, row.WorkspaceID, row.Revision, e.ActiveReceipt}, stillWithinCallDeadline); err != nil {
		return err
	}
	if s.RevokeKey != nil && e.ActiveReceipt.Valid {
		s.revokeAttemptKey(e.ActiveReceipt)
	}
	return nil
}
