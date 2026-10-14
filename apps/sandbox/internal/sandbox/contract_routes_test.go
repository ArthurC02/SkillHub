package sandbox

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	contractPathLine   = regexp.MustCompile(`^  (/\S*):\s*$`)
	contractMethodLine = regexp.MustCompile(`^    (get|put|post|patch|delete):\s*$`)
)

func contractOperations(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "..", "contracts", "openapi", "sandbox-provider.yaml"))
	if err != nil {
		t.Fatalf("the provider contract must be readable from this module: %v", err)
	}
	defer func() { _ = f.Close() }()
	ops := map[string]bool{}
	inPaths, path := false, ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "paths:":
			inPaths = true
		case inPaths && line != "" && !strings.HasPrefix(line, " "):
			inPaths = false
		case inPaths && contractPathLine.MatchString(line):
			path = contractPathLine.FindStringSubmatch(line)[1]
		case inPaths && contractMethodLine.MatchString(line):
			ops[strings.ToUpper(contractMethodLine.FindStringSubmatch(line)[1])+" "+path] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatal("read no operations from the provider contract")
	}
	return ops
}

func servedOperations(s *Server) map[string]bool {
	ops := map[string]bool{}
	for _, r := range s.routeTable() {
		ops[r.pattern] = true
	}
	return ops
}

func missingFrom(want, have map[string]bool) []string {
	var missing []string
	for op := range want {
		if !have[op] {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	return missing
}

func TestEveryRouteTheProviderServesIsInTheContract(t *testing.T) {
	served := servedOperations(&Server{Metrics: prometheus.NewRegistry()})
	if undeclared := missingFrom(served, contractOperations(t)); len(undeclared) > 0 {
		t.Errorf("add these to sandbox-provider.yaml before serving them: %v", undeclared)
	}
}

func TestEveryOperationInTheContractIsServed(t *testing.T) {
	served := servedOperations(&Server{Metrics: prometheus.NewRegistry()})
	if unserved := missingFrom(contractOperations(t), served); len(unserved) > 0 {
		t.Errorf("the contract declares operations this provider does not serve: %v", unserved)
	}
}

func TestMetricsAreServedOnlyWithARegistry(t *testing.T) {
	if servedOperations(&Server{})["GET /metrics"] {
		t.Error("GET /metrics is routed without a metrics registry")
	}
	if !servedOperations(&Server{Metrics: prometheus.NewRegistry()})["GET /metrics"] {
		t.Error("GET /metrics is not routed although a metrics registry was given")
	}
}
