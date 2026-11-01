package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type storedEdge struct {
	From, To, At string
}

var runningChecker = sync.OnceValue(func() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
})

func userCacheDir(purpose string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "skillhub", purpose)
}

func wiringCacheDir() string {
	return userCacheDir("wiring-edges")
}

func cachedWiringEdges(cacheDir, checker, moduleDir string, identities map[string]packageIdentity, compute func() (map[contextEdge]string, error)) (map[contextEdge]string, error) {
	identityJSON, err := json.Marshal(identities)
	if err != nil || checker == "" {
		return compute()
	}
	key, err := moduleKey(moduleDir, checker, runtime.GOOS, runtime.GOARCH, os.Getenv("GOFLAGS"), string(identityJSON))
	if err != nil {
		return compute()
	}
	var edges map[contextEdge]string
	_, err = cachedModuleOutput(cacheDir, key, func(data []byte) bool {
		var ok bool
		edges, ok = decodeStoredEdges(data)
		return ok
	}, func() ([]byte, error) {
		computed, err := compute()
		if err != nil {
			return nil, err
		}
		edges = computed
		return encodeStoredEdges(computed)
	})
	return edges, err
}

func cachedModuleOutput(cacheDir, key string, usable func([]byte) bool, compute func() ([]byte, error)) ([]byte, error) {
	if cacheDir == "" {
		return compute()
	}
	path := filepath.Join(cacheDir, key)
	if data, err := os.ReadFile(path); err == nil && usable(data) {
		return data, nil
	}
	data, err := compute()
	if err != nil {
		return nil, err
	}
	writeCacheEntry(cacheDir, path, data)
	return data, nil
}

func moduleKey(moduleDir string, parts ...string) (string, error) {
	files, err := wiringSources(moduleDir)
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, part := range parts {
		writeHashPart(hash, []byte(part))
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(moduleDir, file)
		writeHashPart(hash, []byte(filepath.ToSlash(rel)))
		writeHashPart(hash, data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func wiringSources(moduleDir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(moduleDir, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.IsDir() && path != moduleDir && isSkippedWiringDir(entry.Name()):
			return filepath.SkipDir
		case !entry.IsDir() && isWiringSource(entry.Name()):
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func isSkippedWiringDir(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func isWiringSource(name string) bool {
	return name == "go.mod" || name == "go.sum" || (strings.HasSuffix(name, extGo) && !strings.HasSuffix(name, "_test.go"))
}

func writeHashPart(hash interface{ Write([]byte) (int, error) }, part []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(part)))
	_, _ = hash.Write(size[:])
	_, _ = hash.Write(part)
}

func decodeStoredEdges(data []byte) (map[contextEdge]string, bool) {
	var stored []storedEdge
	if err := json.Unmarshal(data, &stored); err != nil || stored == nil {
		return nil, false
	}
	edges := make(map[contextEdge]string, len(stored))
	for _, edge := range stored {
		edges[contextEdge{edge.From, edge.To}] = edge.At
	}
	return edges, true
}

func encodeStoredEdges(edges map[contextEdge]string) ([]byte, error) {
	stored := make([]storedEdge, 0, len(edges))
	for edge, at := range edges {
		stored = append(stored, storedEdge{edge.from, edge.to, at})
	}
	sort.Slice(stored, func(i, j int) bool {
		return stored[i].From+"->"+stored[i].To < stored[j].From+"->"+stored[j].To
	})
	return json.Marshal(stored)
}

func writeCacheEntry(cacheDir, path string, data []byte) {
	if os.MkdirAll(cacheDir, 0o755) != nil {
		return
	}
	partial, err := os.CreateTemp(cacheDir, "partial-*")
	if err != nil {
		return
	}
	_, writeErr := partial.Write(data)
	closeErr := partial.Close()
	if writeErr != nil || closeErr != nil || os.Rename(partial.Name(), path) != nil {
		_ = os.Remove(partial.Name())
	}
}
