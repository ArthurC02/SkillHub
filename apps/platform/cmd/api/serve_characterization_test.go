package main

import (
	"os"
	"strings"
	"testing"
)

func TestAListenerThatCannotBindStopsTheAPIWithExitOne(t *testing.T) {
	endpoint := os.Getenv("SKILLHUB_TEST_OBJSTORE_ENDPOINT")
	if endpoint == "" {
		if os.Getenv("SKILLHUB_REQUIRE_OBJSTORE") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_OBJSTORE=1 but SKILLHUB_TEST_OBJSTORE_ENDPOINT is unset")
		}
		t.Skip("SKILLHUB_TEST_OBJSTORE_ENDPOINT not set; the API cannot reach its listener without an object store")
	}
	got := runMain(t, nil, unreachableDatabase, "RATE_LIMIT=off", "COOKIE_INSECURE=1", "API_ADDR=127.0.0.1:99999",
		"OBJSTORE_ENDPOINT="+endpoint, "OBJSTORE_BUCKET=skillhub-presign-test",
		"OBJSTORE_ACCESS_KEY="+envOr("SKILLHUB_TEST_OBJSTORE_ACCESS_KEY", "skillhubdev"),
		"OBJSTORE_SECRET_KEY="+envOr("SKILLHUB_TEST_OBJSTORE_SECRET_KEY", "skillhubdevsecret"))
	if got.code != 1 || !strings.Contains(got.stderr, "api stopped") {
		t.Fatalf("exit %d, want 1 after the listener failed; stderr:\n%s", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "api listening") {
		t.Errorf("the API never reached its listener, so this test did not exercise the serve path:\n%s", got.stderr)
	}
	if strings.Contains(got.stderr, webAppCapabilityName) || !strings.Contains(got.stderr, "CREATION_WORKER_INTERNAL_ADDR") {
		t.Errorf("a deployment that serves no build reported the clean-mode capability table:\n%s", got.stderr)
	}
}

const webAppCapabilityName = "網頁介面（這個行程送出的 SPA）"

func TestCleanModeReportsTheWebBuildAndNoWorkerListener(t *testing.T) {
	got := runMain(t, nil, unreachableDatabase, "SKILLHUB_CLEAN_MODE=1", "RATE_LIMIT=off", "COOKIE_INSECURE=1")
	if got.code != 1 || !strings.Contains(got.stderr, "clean mode: queue schema") {
		t.Fatalf("exit %d, want 1 at the queue schema after the report; stderr:\n%s", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, webAppCapabilityName) {
		t.Errorf("clean mode did not report the web build it serves:\n%s", got.stderr)
	}
	if strings.Contains(got.stderr, "CREATION_WORKER_INTERNAL_ADDR") {
		t.Errorf("clean mode asked for the separate worker's listener:\n%s", got.stderr)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
