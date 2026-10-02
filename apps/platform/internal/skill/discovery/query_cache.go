package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"maps"
	"slices"
	"sync"
	"time"
)

const (
	cacheTTL     = time.Hour
	cacheEntries = 10000
)

type cachedAnswer[V any] struct {
	value   V
	expires time.Time
}

type ttlCache[V any] struct {
	mu      sync.Mutex
	entries map[string]cachedAnswer[V]
	now     func() time.Time
}

func (c *ttlCache[V]) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !c.clock().Before(entry.expires) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *ttlCache[V]) put(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.entries == nil {
		c.entries = map[string]cachedAnswer[V]{}
	}
	if len(c.entries) >= cacheEntries {
		for k, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, k)
			}
		}
	}
	for k := range c.entries {
		if len(c.entries) < cacheEntries {
			break
		}
		delete(c.entries, k)
	}
	c.entries[key] = cachedAnswer[V]{value: value, expires: now.Add(cacheTTL)}
}

type cachedModel struct {
	Model
	embeddings ttlCache[Embeddings]
	reasons    ttlCache[MatchReasons]
}

type cachedIntentAnalyzer struct {
	IntentAnalyzer
	analyses ttlCache[IntentAnalysis]
}

func (m *cachedModel) Embed(ctx context.Context, texts []string, within time.Duration) (*Embeddings, error) {
	if len(texts) != 1 {
		return m.Model.Embed(ctx, texts, within)
	}
	if hit, ok := m.embeddings.get(texts[0]); ok {
		hit.Vectors, hit.Usage = slices.Clone(hit.Vectors), reusedAnswer()
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
		hit.Reasons, hit.Usage = slices.Clone(hit.Reasons), reusedAnswer()
		return &hit, nil
	}
	answer, err := m.Model.MatchReasons(ctx, query, candidates, within)
	if err == nil && answer != nil {
		m.reasons.put(key, *answer)
	}
	return answer, err
}

func reusedAnswer() *ModelUsage { return &ModelUsage{Reused: true} }

func reasonsKey(query string, candidates []SkillCandidate) string {
	h := sha256.New()
	writeField := func(field string) {
		h.Write(binary.AppendUvarint(nil, uint64(len(field))))
		h.Write([]byte(field))
	}
	writeField(query)
	for _, c := range candidates {
		writeField(c.SkillID)
		writeField(c.Name)
		writeField(c.Summary)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *cachedIntentAnalyzer) AnalyzeIntent(ctx context.Context, query string, within time.Duration) (*IntentAnalysis, error) {
	if hit, ok := a.analyses.get(query); ok {
		hit.Interpretation = hit.Interpretation.clone()
		hit.Usage = reusedAnswer()
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
