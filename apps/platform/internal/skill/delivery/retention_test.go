package packaging

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestCreateFailsClosedWithoutRetention(t *testing.T) {
	service := &Service{}
	_, err := service.Create(context.Background(), identity.Workspace{}, pgtype.UUID{}, pgtype.UUID{}, "standard", false)
	if !errors.Is(err, ErrRetentionNotConfigured) {
		t.Fatalf("Create() error = %v, want ErrRetentionNotConfigured", err)
	}
}

func TestAnUnconfiguredDeploymentRefusesInChinese(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"no profile for the target", ErrNoProfile, "沒有設定這個打包目標"},
		{"no object store", ErrNoStore, "沒有接上套件儲存"},
		{"no ratified retention", ErrRetentionNotConfigured, "沒有設定套件的保存期限"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if (&Handler{}).writeServiceError(w, tc.err) {
				t.Fatal("writeServiceError reported success for a refusal")
			}
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), tc.want)
			}
			if strings.Contains(w.Body.String(), tc.err.Error()) {
				t.Errorf("body = %q, still carries the English sentence meant for the log", w.Body.String())
			}
		})
	}
}
