package creation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	watchTick              = 250 * time.Millisecond
	MaxStreamsPerWorkspace = 4
	MaxStreamsPerProcess   = 2000
)

var ErrTooManyStreams = errors.New("creation: too many open streams")

type watchKey struct {
	workspace [16]byte
	session   [16]byte
}

type revisionReader func(ctx context.Context, workspaces, sessions []pgtype.UUID) ([]gen.CreationSessionRevisionsRow, error)

type RevisionWatch struct {
	read revisionReader
	tick time.Duration

	mu           sync.Mutex
	subscribers  map[watchKey]map[chan struct{}]struct{}
	perWorkspace map[[16]byte]int
	total        int
	seen         map[watchKey]int64
	running      bool
}

func NewRevisionWatch(pool *pgxpool.Pool) *RevisionWatch {
	return newRevisionWatch(func(ctx context.Context, workspaces, sessions []pgtype.UUID) ([]gen.CreationSessionRevisionsRow, error) {
		return gen.New(pool).CreationSessionRevisions(ctx, gen.CreationSessionRevisionsParams{SessionIds: sessions, WorkspaceIds: workspaces})
	}, watchTick)
}

func newRevisionWatch(read revisionReader, tick time.Duration) *RevisionWatch {
	return &RevisionWatch{
		read: read, tick: tick,
		subscribers:  map[watchKey]map[chan struct{}]struct{}{},
		perWorkspace: map[[16]byte]int{},
		seen:         map[watchKey]int64{},
	}
}

func (w *RevisionWatch) Subscribe(workspace, session pgtype.UUID) (<-chan struct{}, func(), error) {
	key := watchKey{workspace: workspace.Bytes, session: session.Bytes}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.total >= MaxStreamsPerProcess || w.perWorkspace[key.workspace] >= MaxStreamsPerWorkspace {
		return nil, nil, ErrTooManyStreams
	}
	changed := make(chan struct{}, 1)
	changed <- struct{}{}
	if w.subscribers[key] == nil {
		w.subscribers[key] = map[chan struct{}]struct{}{}
	}
	w.subscribers[key][changed] = struct{}{}
	w.perWorkspace[key.workspace]++
	w.total++
	if !w.running {
		w.running = true
		go w.watch()
	}
	var once sync.Once
	return changed, func() { once.Do(func() { w.unsubscribe(key, changed) }) }, nil
}

func (w *RevisionWatch) unsubscribe(key watchKey, changed chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.subscribers[key], changed)
	if len(w.subscribers[key]) == 0 {
		delete(w.subscribers, key)
		delete(w.seen, key)
	}
	w.perWorkspace[key.workspace]--
	if w.perWorkspace[key.workspace] == 0 {
		delete(w.perWorkspace, key.workspace)
	}
	w.total--
}

func (w *RevisionWatch) watch() {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for range ticker.C {
		if !w.poll() {
			return
		}
	}
}

func (w *RevisionWatch) poll() bool {
	keys, ok := w.watchedKeys()
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*w.tick)
	defer cancel()
	workspaces, sessions := make([]pgtype.UUID, len(keys)), make([]pgtype.UUID, len(keys))
	for i, key := range keys {
		workspaces[i] = pgtype.UUID{Bytes: key.workspace, Valid: true}
		sessions[i] = pgtype.UUID{Bytes: key.session, Valid: true}
	}
	rows, err := w.read(ctx, workspaces, sessions)
	if err != nil {
		return true
	}
	current := make(map[watchKey]int64, len(rows))
	for _, row := range rows {
		current[watchKey{workspace: row.WorkspaceID.Bytes, session: row.ID.Bytes}] = row.Revision
	}
	w.notifyChanged(current)
	return true
}

func (w *RevisionWatch) watchedKeys() ([]watchKey, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.total == 0 {
		w.running = false
		return nil, false
	}
	keys := make([]watchKey, 0, len(w.subscribers))
	for key := range w.subscribers {
		keys = append(keys, key)
	}
	return keys, true
}

// A session missing from current was deleted, so its subscribers are told on
// every tick until they read it, find it gone and close.
func (w *RevisionWatch) notifyChanged(current map[watchKey]int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for key, subscribers := range w.subscribers {
		revision, present := current[key]
		last, known := w.seen[key]
		if present && known && revision == last {
			continue
		}
		if present {
			w.seen[key] = revision
		}
		for changed := range subscribers {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
	}
}
