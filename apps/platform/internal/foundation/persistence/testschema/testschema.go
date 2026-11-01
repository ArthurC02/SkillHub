package testschema

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	templatePrefix = "skillhub_test_tpl_"
	clonePrefix    = "skillhub_test_clone_"
)

func MustPrepare(ctx context.Context, dsn string) (clone string, drop func()) {
	clone, drop, err := Prepare(ctx, dsn)
	if err != nil {
		panic(err)
	}
	return clone, drop
}

// Prepare clones a template holding every migration into a database private to the calling test binary.
// drop removes that clone; call it after closing every pool that points at it.
func Prepare(ctx context.Context, dsn string) (clone string, drop func(), err error) {
	if err := ValidateDestructiveURL(dsn); err != nil {
		return "", nil, err
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", nil, err
	}
	defer admin.Close(ctx)
	template, err := ensureTemplate(ctx, admin)
	if err != nil {
		return "", nil, err
	}
	name := clonePrefix + randomSuffix()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quote(name)+" TEMPLATE "+quote(template)); err != nil {
		return "", nil, fmt.Errorf("clone %s from %s: %w", name, template, err)
	}
	drop = func() {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, dsn)
		if err != nil {
			return
		}
		defer conn.Close(ctx)
		_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+quote(name)+" WITH (FORCE)")
	}
	return withDatabase(dsn, name), drop, nil
}

func ValidateDestructiveURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("test database URL is invalid: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("test database URL must target localhost before databases are created and dropped")
	}
	if !strings.HasSuffix(strings.ToLower(strings.Trim(u.Path, "/")), "_test") {
		return fmt.Errorf("test database name must end in _test before databases are created and dropped")
	}
	return nil
}

func ensureTemplate(ctx context.Context, admin *pgx.Conn) (string, error) {
	migrations, hash, err := readMigrations()
	if err != nil {
		return "", err
	}
	template := templatePrefix + hash
	unlock, err := lock(ctx, admin)
	if err != nil {
		return "", err
	}
	defer unlock()
	var exists bool
	if err := admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", template).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return template, nil
	}
	if err := buildTemplate(ctx, admin, template, migrations); err != nil {
		return "", err
	}
	dropStaleTemplates(ctx, admin, template)
	return template, nil
}

func buildTemplate(ctx context.Context, admin *pgx.Conn, template string, migrations []string) error {
	building := template + "_building"
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quote(building)+" WITH (FORCE)"); err != nil {
		return err
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quote(building)); err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, withDatabase(admin.Config().ConnString(), building))
	if err != nil {
		return err
	}
	for _, body := range migrations {
		if _, err := conn.Exec(ctx, body); err != nil {
			conn.Close(ctx)
			return err
		}
	}
	if err := conn.Close(ctx); err != nil {
		return err
	}
	_, err = admin.Exec(ctx, "ALTER DATABASE "+quote(building)+" RENAME TO "+quote(template))
	return err
}

func dropStaleTemplates(ctx context.Context, admin *pgx.Conn, current string) {
	rows, err := admin.Query(ctx, "SELECT datname FROM pg_database WHERE datname LIKE $1 AND datname <> $2", templatePrefix+"%", current)
	if err != nil {
		return
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return
	}
	for _, name := range stale {
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quote(name))
	}
}

func readMigrations() (bodies []string, hash string, err error) {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, "", err
		}
		digest.Write([]byte(name))
		digest.Write(body)
		bodies = append(bodies, string(body))
	}
	return bodies, hex.EncodeToString(digest.Sum(nil))[:12], nil
}

func lock(ctx context.Context, conn *pgx.Conn) (unlock func(), err error) {
	if err := gen.New(conn).LockTestSchema(ctx); err != nil {
		return nil, err
	}
	return func() {
		_ = gen.New(conn).UnlockTestSchema(context.WithoutCancel(ctx))
	}, nil
}

func withDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + name
	return u.String()
}

func quote(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

func randomSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
