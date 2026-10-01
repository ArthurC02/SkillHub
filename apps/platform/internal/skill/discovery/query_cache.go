package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"sync"
	"time"
)

const (
	queryCacheTTL     = time.Hour
	queryCacheEntries = 10000
)

type cachedAnswer[V any] struct {
	value   V
	expires time.Time
}

type queryCache[V any] struct {
	mu      sync.Mutex
	entries map[string]cachedAnswer[V]
	now     func() time.Time
}

func (c *queryCache[V]) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func (c *queryCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !c.clock().Before(entry.expires) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *queryCache[V]) put(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.entries == nil {
		c.entries = map[string]cachedAnswer[V]{}
	}
	if len(c.entries) >= queryCacheEntries {
		for k, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, k)
			}
		}
	}
	for k := range c.entries {
		if len(c.entries) < queryCacheEntries {
			break
		}
		delete(c.entries, k)
	}
	c.entries[key] = cachedAnswer[V]{value: value, expires: now.Add(queryCacheTTL)}
}

type cachedModel struct {
	Model
	embeddings queryCache[Embeddings]
	reasons    queryCache[MatchReasons]
}

type cachedIntentAnalyzer struct {
	IntentAnalyzer
	analyses queryCache[IntentAnalysis]
}

func (m *cachedModel) Embed(ctx context.Context, texts []string, within time.Duration) (*Embeddings, error) {
	if len(texts) != 1 {
		return m.Model.Embed(ctx, texts, within)
	}
	if hit, ok := m.embeddings.get(texts[0]); ok {
		hit.Vectors, hit.Usage = slices.Clone(hit.Vectors), nil
		return &hit, nil
	}
	answer, err := m.Model.Embed(ctx, texts, within)
	if err == nil && answer != nil && len(answer.Vectors) == 1 {
		m.embeddings.put(texts[0], *answer)
	}
	return answer, err
}

func (m *cachedModel) MatchReasons(ctx context.Context, query string, candidates []SkillCandidate, within time.Duration) (*MatchReasons, error) {
	key := reasonsKey(query, candidates)
	if hit, ok := m.reasons.get(key); ok {
		hit.Reasons, hit.Usage = slices.Clone(hit.Reasons), nil
		return &hit, nil
	}
	answer, err := m.Model.MatchReasons(ctx, query, candidates, within)
	if err == nil && answer != nil {
		m.reasons.put(key, *answer)
	}
	return answer, err
}

func reasonsKey(query string, candidates []SkillCandidate) string {
	h := sha256.New()
	h.Write([]byte(query))
	for _, c := range candidates {
		for _, field := range []string{c.SkillID, c.Name, c.Summary} {
			h.Write([]byte{0})
			h.Write([]byte(field))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *cachedIntentAnalyzer) AnalyzeIntent(ctx context.Context, query string, within time.Duration) (*IntentAnalysis, error) {
	if hit, ok := a.analyses.get(query); ok {
		hit.Interpretation = hit.Interpretation.clone()
		hit.Usage = nil
		return &hit, nil
	}
	answer, err := a.IntentAnalyzer.AnalyzeIntent(ctx, query, within)
	if err == nil && answer != nil && answer.Valid {
		stored := *answer
		stored.Interpretation = answer.Interpretation.clone()
		a.analyses.put(query, stored)
	}
	return answer, err
}

func (i SearchInterpretation) clone() SearchInterpretation {
	i.Intent = maps.Clone(i.Intent)
	i.Filters = maps.Clone(i.Filters)
	i.Keywords = slices.Clone(i.Keywords)
	return i
}
