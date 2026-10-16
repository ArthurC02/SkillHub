package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

var registryAsset = exportedRegistryAsset

var exportedRegistries = struct {
	sync.Mutex
	byRoot map[string]map[string]json.RawMessage
}{byRoot: map[string]map[string]json.RawMessage{}}

func exportedRegistryAsset(root, asset string) ([]byte, error) {
	exportedRegistries.Lock()
	defer exportedRegistries.Unlock()
	assets, ok := exportedRegistries.byRoot[root]
	if !ok {
		exported, err := exportRegistry(root)
		if err != nil {
			return nil, err
		}
		assets = exported
		exportedRegistries.byRoot[root] = assets
	}
	document, ok := assets[asset]
	if !ok {
		return nil, fmt.Errorf("the Domain Memory export holds no %s asset", asset)
	}
	return document, nil
}

func exportRegistry(root string) (map[string]json.RawMessage, error) {
	output, runErr := domainMemoryTool(root, "export", "--registry-root", filepath.Join(root, filepath.FromSlash(domainMemoryRoot)))
	if runErr != nil {
		return nil, fmt.Errorf("the Domain Memory export failed: %w: %s", runErr, firstLine(output))
	}
	var assets map[string]json.RawMessage
	start := strings.Index(output, "{")
	if start < 0 {
		return nil, fmt.Errorf("the Domain Memory export printed no JSON: %s", firstLine(output))
	}
	if err := json.Unmarshal([]byte(output[start:]), &assets); err != nil {
		return nil, fmt.Errorf("the Domain Memory export is not JSON: %w", err)
	}
	return assets, nil
}
