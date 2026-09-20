package objstoretest

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"
)

type InMemory struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func New() *InMemory { return &InMemory{objects: map[string][]byte{}} }

func (s *InMemory) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = append([]byte(nil), data...)
	return nil
}

func (s *InMemory) GetIfPresent(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, held := s.objects[key]
	if !held {
		return nil, false, nil
	}
	return append([]byte(nil), data...), true, nil
}

func (s *InMemory) Get(ctx context.Context, key string) ([]byte, error) {
	data, found, err := s.GetIfPresent(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("objstoretest get %s: no object with that key", key)
	}
	return data, nil
}

func (s *InMemory) Exists(ctx context.Context, key string) (bool, error) {
	_, found, err := s.GetIfPresent(ctx, key)
	return found, err
}

func (s *InMemory) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *InMemory) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return presigned("get", key, ttl), nil
}

func (s *InMemory) PresignPut(_ context.Context, key string, ttl time.Duration) (string, error) {
	return presigned("put", key, ttl), nil
}

func presigned(direction, key string, ttl time.Duration) string {
	return "https://objstore.test/" + direction + "/" + url.PathEscape(key) +
		"?expires_in=" + fmt.Sprint(int(ttl.Seconds()))
}
