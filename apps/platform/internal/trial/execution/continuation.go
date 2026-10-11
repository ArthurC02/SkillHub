package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

const (
	DefaultContinuationRounds = 3
	MaxAnswerBytes            = 2000
)

var (
	ErrNothingToAnswer      = errors.New("this run did not end by asking the person anything")
	ErrAnswersDoNotFit      = errors.New("every question needs one non-blank answer of at most 2000 bytes")
	ErrAlreadyContinued     = errors.New("this run's questions have already been answered")
	ErrContinuationLimit    = errors.New("this conversation has reached its limit of answered rounds")
	ErrContinuationTooLarge = errors.New("the request with every answer so far is longer than a prompt may be")
)

type ContinueParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	RunID       pgtype.UUID
	Answers     []string
}

type answeredQuestion struct {
	trace.Question
	Answer string
}

func (s *Service) Continue(ctx context.Context, p ContinueParams) (RunView, error) {
	if err := s.requireTestLab(); err != nil {
		return RunView{}, err
	}
	original, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: p.RunID, WorkspaceID: p.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return RunView{}, ErrNotFound
	}
	if err != nil {
		return RunView{}, err
	}
	answered, err := s.answeredQuestions(ctx, original, p.Answers)
	if err != nil {
		return RunView{}, err
	}
	snapshot, err := s.TestLab.ReadSnapshot(ctx, p.WorkspaceID, original.TestCaseSnapshotID)
	if err != nil {
		return RunView{}, err
	}
	prompt := continuedPrompt(snapshot.UserPrompt, answered)

	version, found, err := s.Registry.Version(ctx, p.WorkspaceID, original.SkillVersionID)
	if err != nil {
		return RunView{}, err
	}
	if !found {
		return RunView{}, ErrNotFound
	}
	params := CreateParams{
		WorkspaceID: p.WorkspaceID, Actor: p.Actor,
		SkillID: version.SkillID, VersionID: version.ID, TestCaseID: snapshot.TestCaseID,
	}
	run, err := s.continueRun(ctx, params, continuation{original: original, answered: answered, actor: p.Actor}, prompt)
	if err != nil {
		s.auditRefusal(ctx, params, err)
	}
	return runView(run), err
}

func (s *Service) answeredQuestions(ctx context.Context, original gen.Run, answers []string) ([]answeredQuestion, error) {
	if original.Status != gen.RunStatusSucceeded || s.Trace == nil {
		return nil, ErrNothingToAnswer
	}
	summary, err := s.Trace.General(ctx, original.WorkspaceID, original.ID)
	if err != nil {
		return nil, err
	}
	if len(summary.Questions) == 0 {
		return nil, ErrNothingToAnswer
	}
	return pairAnswers(summary.Questions, answers)
}

func pairAnswers(questions []trace.Question, answers []string) ([]answeredQuestion, error) {
	if len(answers) != len(questions) {
		return nil, ErrAnswersDoNotFit
	}
	out := make([]answeredQuestion, 0, len(questions))
	for i, q := range questions {
		answer := strings.TrimSpace(answers[i])
		if answer == "" || len(answer) > MaxAnswerBytes {
			return nil, ErrAnswersDoNotFit
		}
		out = append(out, answeredQuestion{Question: q, Answer: answer})
	}
	return out, nil
}

func continuedPrompt(original string, answered []answeredQuestion) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(original, "\n"))
	b.WriteString("\n\n你先前提出的問題與我的回答：")
	for i, a := range answered {
		fmt.Fprintf(&b, "\n%d. 問：%s\n   答：%s", i+1, a.Question.Question, a.Answer)
	}
	return b.String()
}

type continuation struct {
	original gen.Run
	answered []answeredQuestion
	actor    pgtype.UUID
}

func (s *Service) continueRun(ctx context.Context, p CreateParams, c continuation, prompt string) (gen.Run, error) {
	original := c.original
	admitted, err := s.admitRun(ctx, p)
	if err != nil {
		return gen.Run{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := s.requireOpenRound(ctx, q, original); err != nil {
		return gen.Run{}, err
	}
	if err := s.reserveRunCapacity(ctx, tx, q, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}
	snapshot, err := s.TestLab.ContinueSnapshot(ctx, tx, p.WorkspaceID, original.TestCaseSnapshotID, prompt)
	if errors.Is(err, testlab.ErrPromptTooLong) {
		return gen.Run{}, ErrContinuationTooLarge
	}
	if err != nil {
		return gen.Run{}, err
	}
	run, err := s.launch(ctx, tx, p, admitted, snapshot.ID)
	if err != nil {
		return gen.Run{}, err
	}
	if err := c.record(ctx, q, run); err != nil {
		return gen.Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Run{}, err
	}
	metrics.RunCreated.Inc()
	return run, nil
}

func (s *Service) requireOpenRound(ctx context.Context, q *gen.Queries, original gen.Run) error {
	continued, err := q.RunWasContinued(ctx, gen.RunWasContinuedParams{
		RunID: original.ID, WorkspaceID: original.WorkspaceID,
	})
	if err != nil {
		return err
	}
	if continued {
		return ErrAlreadyContinued
	}
	rounds, err := s.ContinuationRounds(ctx)
	if err != nil {
		return err
	}
	depth, err := q.CountContinuationChain(ctx, gen.CountContinuationChainParams{
		RunID: original.ID, WorkspaceID: original.WorkspaceID,
	})
	if err != nil {
		return err
	}
	if int(depth) >= rounds {
		return ErrContinuationLimit
	}
	return nil
}

func (c continuation) record(ctx context.Context, q *gen.Queries, run gen.Run) error {
	questions := make([]trace.Question, 0, len(c.answered))
	answers := make([]string, 0, len(c.answered))
	for _, a := range c.answered {
		questions = append(questions, a.Question)
		answers = append(answers, a.Answer)
	}
	encodedQuestions, err := json.Marshal(questions)
	if err != nil {
		return err
	}
	encodedAnswers, err := json.Marshal(answers)
	if err != nil {
		return err
	}
	_, err = q.InsertRunContinuation(ctx, gen.InsertRunContinuationParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID, ContinuesRunID: c.original.ID,
		Questions: encodedQuestions, Answers: encodedAnswers, AnsweredBy: c.actor,
	})
	if pgconv.IsUniqueViolation(err) {
		return ErrAlreadyContinued
	}
	return err
}
