package ingest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

const creationDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var creationPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(creationDBURLEnv)
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", creationDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	if err := validateDestructiveCreationDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockCreationTestSchema(ctx, pool)
	if err := migrateCreationSchema(ctx, pool); err != nil {
		panic(err)
	}
	creationPool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

func validateDestructiveCreationDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", creationDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", creationDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", creationDBURLEnv)
	}
	return nil
}

func migrateCreationSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func lockCreationTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	return func() {
		_, _ = conn.Exec(ctx,
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	}
}

func requireCreationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if creationPool == nil {
		t.Skipf("%s not set; skipping MaterializeGeneratedCandidate database test", creationDBURLEnv)
	}
	return creationPool
}

func seedCreationWorkspace(t *testing.T, pool *pgxpool.Pool, name string) identity.Workspace {
	t.Helper()
	ctx := context.Background()
	var ws identity.Workspace
	var userID pgtype.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name) VALUES ($1, $1) RETURNING id`,
		name+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO workspaces (owner_user_id, name) VALUES ($1, $2)
		 RETURNING id, owner_user_id, name, created_at, updated_at, is_catalog`,
		userID, name).Scan(&ws.ID, &ws.OwnerUserID, &ws.Name, &ws.CreatedAt, &ws.UpdatedAt, &ws.IsCatalog); err != nil {
		t.Fatal(err)
	}
	return ws
}

type creationTestStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *creationTestStore) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string][]byte{}
	}
	s.data[key] = append([]byte(nil), data...)
	return nil
}

func (s *creationTestStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key], nil
}

func TestCreationCandidateMaterializeDuplicateIsReuseNotError(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "creation-dup-fixture")

	svc := &Service{
		Pool:  pool,
		Store: &creationTestStore{},
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error {
			return nil
		},
	}

	skill := goodGeneratedSkill()
	prov := GeneratedCandidateProvenance{TaskDescription: "test task", Model: "test-model", PromptVersion: "v1"}

	first, err := svc.MaterializeGeneratedCandidate(ctx, ws, skill, prov, nil)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first materialize reported Duplicate; nothing existed yet")
	}

	prov.ExistingSkillID = &first.Skill.ID
	second, err := svc.MaterializeGeneratedCandidate(ctx, ws, skill, prov, nil)
	if err != nil {
		t.Fatalf("second materialize of identical content returned an error instead of reusing the version: %v", err)
	}
	if !second.Duplicate {
		t.Error("second materialize of identical content was not reported as a duplicate")
	}
	if second.Version.ID != first.Version.ID {
		t.Errorf("second materialize Version.ID = %v, want the existing version %v", second.Version.ID, first.Version.ID)
	}
	if second.Skill.ID != first.Skill.ID {
		t.Errorf("second materialize Skill.ID = %v, want %v", second.Skill.ID, first.Skill.ID)
	}
}

func TestCreationRevisionEnrichesWithoutHoldingTheOnlyConnection(t *testing.T) {
	for _, scenario := range []string{"owned", "foreign", "uploaded"} {
		t.Run(scenario, func(t *testing.T) {
			shared := requireCreationDB(t)
			ws := seedCreationWorkspace(t, shared, "creation-enrich-"+scenario)
			cfg := shared.Config()
			cfg.MaxConns, cfg.MinConns = 1, 0
			pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			var projection SkillProjection
			svc := &Service{Pool: pool, Store: &creationTestStore{}, IndexSkill: func(_ context.Context, _ pgx.Tx, p SkillProjection) error {
				projection = p
				return nil
			}}
			skill := goodGeneratedSkill()
			prov := GeneratedCandidateProvenance{TaskDescription: "test task", Model: "test-model", PromptVersion: "v1"}
			var first Result
			if scenario == "uploaded" {
				data, buildErr := buildGeneratedPackage(skill)
				if buildErr != nil {
					t.Fatal(buildErr)
				}
				first, err = svc.importZip(t.Context(), ws, data, sourceMeta{Type: SourceUpload})
			} else {
				first, err = svc.MaterializeGeneratedCandidate(t.Context(), ws, skill, prov, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "foreign" {
				ws = seedCreationWorkspace(t, shared, "creation-enrich-other")
			}
			stub := &stubEnricher{enrichStatus: http.StatusOK, embedStatus: http.StatusOK}
			svc.LLM = stub.start(t)
			svc.Budgets = &modelbudget.Service{Pool: pool}
			prov.ExistingSkillID = &first.Skill.ID
			skill.Body += "\nReturn a concise summary.\n"
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			got, err := svc.MaterializeGeneratedCandidate(ctx, ws, skill, prov, nil)
			if scenario != "owned" {
				if !errors.Is(err, ErrGeneratedNameCollision) || len(stub.embedded) != 0 {
					t.Fatalf("unauthorized revision: error=%v, embedded=%d", err, len(stub.embedded))
				}
				return
			}
			if err != nil {
				t.Fatalf("single-connection revision: %v", err)
			}
			if got.Skill.ID != first.Skill.ID || got.Version.ID == first.Version.ID || got.Version.VersionNumber != first.Version.VersionNumber+1 || got.Duplicate {
				t.Fatalf("revision did not create the next version: %+v", got)
			}
			if len(stub.embedded) != 1 || projection.EnrichmentStatus != "enriched" || projection.EnrichedSummary != testEnrichedSummary {
				t.Fatalf("revision lost enrichment: %+v", projection)
			}
		})
	}
}
