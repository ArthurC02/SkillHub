package activity

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestListCombinesRunAndEvaluationByPriorityAndLatestOwnerTime(t *testing.T) {
	runTime := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	evaluationTime := runTime.Add(time.Hour)
	service := completeService()
	service.ReadRuns = func(context.Context, pgtype.UUID) ([]RunFact, error) {
		return []RunFact{{
			RunID: "run-1", SkillName: "Release notes", Classification: Recent,
			Status: Status{Value: "succeeded", Label: "試跑完成"}, ActivityAt: runTime,
		}}, nil
	}
	service.ReadEvaluations = func(context.Context, pgtype.UUID) ([]EvaluationFact, error) {
		return []EvaluationFact{{
			RunID: "run-1", Classification: NeedsAttention,
			Status: Status{Value: "not_met", Label: "未符合驗收標準"}, ActivityAt: evaluationTime,
		}}, nil
	}

	page, err := service.List(context.Background(), pgtype.UUID{}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.Classification != NeedsAttention || item.Status.Value != "not_met" || !item.ActivityAt.Equal(evaluationTime) {
		t.Fatalf("combined item = %+v", item)
	}
}

func TestListUsesTheGlobalTotalOrderAcrossPages(t *testing.T) {
	shared := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	service := completeService()
	service.ReadRuns = func(context.Context, pgtype.UUID) ([]RunFact, error) {
		return []RunFact{
			{RunID: "b", SkillName: "B", Classification: Recent, ActivityAt: shared},
			{RunID: "a", SkillName: "A", Classification: Recent, ActivityAt: shared},
		}, nil
	}
	service.ReadCreations = func(context.Context, pgtype.UUID) ([]CreationFact, error) {
		return []CreationFact{{SessionID: "c", Summary: "C", Classification: NeedsAttention, ActivityAt: shared.Add(-time.Hour)}}, nil
	}

	first, err := service.List(context.Background(), pgtype.UUID{}, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.List(context.Background(), pgtype.UUID{}, first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{first.Items[0].SourceID, first.Items[1].SourceID, second.Items[0].SourceID}
	want := []string{"c", "a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if first.NextCursor == "" || second.NextCursor != "" {
		t.Fatalf("cursors = %q, %q", first.NextCursor, second.NextCursor)
	}
}

func TestListFailsClosedAndNamesEveryUnavailableSource(t *testing.T) {
	service := completeService()
	service.ReadEvaluations = func(context.Context, pgtype.UUID) ([]EvaluationFact, error) {
		return nil, errors.New("evaluation store unavailable")
	}
	service.ReadPublishing = nil

	page, err := service.List(context.Background(), pgtype.UUID{}, "", 50)
	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error = %v", err)
	}
	want := []Source{SourceEvaluation, SourcePublishing}
	if !reflect.DeepEqual(unavailable.Sources, want) {
		t.Fatalf("sources = %v, want %v", unavailable.Sources, want)
	}
	if len(page.Items) != 0 || page.NextCursor != "" || page.Complete {
		t.Fatalf("partial page escaped: %+v", page)
	}
}

func TestListRejectsMalformedCursorBeforeReadingOwners(t *testing.T) {
	service := completeService()
	read := false
	service.ReadRuns = func(context.Context, pgtype.UUID) ([]RunFact, error) {
		read = true
		return nil, nil
	}
	_, err := service.List(context.Background(), pgtype.UUID{}, "not-a-cursor", 50)
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("error = %v, want invalid cursor", err)
	}
	if read {
		t.Fatal("owner reader was called for a malformed cursor")
	}
}

func TestListReturnsAnExplicitCompleteEmptyPage(t *testing.T) {
	page, err := completeService().List(context.Background(), pgtype.UUID{}, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Complete || len(page.Items) != 0 || !reflect.DeepEqual(page.Sources, Sources) {
		t.Fatalf("page = %+v", page)
	}
}

func TestListScopesEveryOwnerReaderToTheRequestedWorkspace(t *testing.T) {
	alice := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	bob := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	seen := map[Source][]pgtype.UUID{}
	service := &Service{
		ReadRuns: func(_ context.Context, workspaceID pgtype.UUID) ([]RunFact, error) {
			seen[SourceRun] = append(seen[SourceRun], workspaceID)
			if workspaceID == alice {
				return []RunFact{{RunID: "alice-run", SkillName: "Alice", Classification: Recent}}, nil
			}
			return nil, nil
		},
		ReadEvaluations: func(_ context.Context, workspaceID pgtype.UUID) ([]EvaluationFact, error) {
			seen[SourceEvaluation] = append(seen[SourceEvaluation], workspaceID)
			if workspaceID == alice {
				return []EvaluationFact{{RunID: "alice-run", Classification: Recent}}, nil
			}
			return nil, nil
		},
		ReadCreations: func(_ context.Context, workspaceID pgtype.UUID) ([]CreationFact, error) {
			seen[SourceCreation] = append(seen[SourceCreation], workspaceID)
			if workspaceID == alice {
				return []CreationFact{{SessionID: "alice-creation", Classification: Recent}}, nil
			}
			return nil, nil
		},
		ReadPackaging: func(_ context.Context, workspaceID pgtype.UUID) ([]PackagingFact, error) {
			seen[SourcePackaging] = append(seen[SourcePackaging], workspaceID)
			if workspaceID == alice {
				return []PackagingFact{{ArtifactID: "alice-artifact", Classification: Recent}}, nil
			}
			return nil, nil
		},
		ReadPublishing: func(_ context.Context, workspaceID pgtype.UUID) ([]PublishingFact, error) {
			seen[SourcePublishing] = append(seen[SourcePublishing], workspaceID)
			if workspaceID == alice {
				return []PublishingFact{{PublicationID: "alice-publication", Classification: Recent}}, nil
			}
			return nil, nil
		},
	}

	alicePage, err := service.List(context.Background(), alice, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	bobPage, err := service.List(context.Background(), bob, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(alicePage.Items) != 4 || len(bobPage.Items) != 0 {
		t.Fatalf("alice items = %d, bob items = %d", len(alicePage.Items), len(bobPage.Items))
	}
	for _, source := range Sources {
		if !reflect.DeepEqual(seen[source], []pgtype.UUID{alice, bob}) {
			t.Errorf("%s workspaces = %v", source, seen[source])
		}
	}
}

func completeService() *Service {
	return &Service{
		ReadRuns:        func(context.Context, pgtype.UUID) ([]RunFact, error) { return nil, nil },
		ReadEvaluations: func(context.Context, pgtype.UUID) ([]EvaluationFact, error) { return nil, nil },
		ReadCreations:   func(context.Context, pgtype.UUID) ([]CreationFact, error) { return nil, nil },
		ReadPackaging:   func(context.Context, pgtype.UUID) ([]PackagingFact, error) { return nil, nil },
		ReadPublishing:  func(context.Context, pgtype.UUID) ([]PublishingFact, error) { return nil, nil },
	}
}
