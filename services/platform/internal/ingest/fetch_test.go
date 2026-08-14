package ingest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFetchRejectsDisallowedHost(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	for _, u := range []string{
		"https://evil.example.com/pkg.zip",
		"https://localhost:8080/internal.zip",
		"http://github.com/o/r", // https required without AllowInsecure
		"://bad",
	} {
		if _, _, err := f.Fetch(context.Background(), u); !errors.Is(err, ErrFetch) {
			t.Fatalf("%s: want ErrFetch, got %v", u, err)
		}
	}
}

func TestFetchRedirectOffAllowListBlocked(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("redirect target must never be reached")
	}))
	defer evil.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/pkg.zip", http.StatusFound)
	}))
	defer src.Close()

	srcHost := strings.TrimPrefix(src.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{srcHost: true}, AllowInsecure: true}
	if _, _, err := f.Fetch(context.Background(), src.URL+"/pkg.zip"); !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch on off-list redirect, got %v", err)
	}
}

func TestFetchSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, MaxZipBytes+1))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	if _, _, err := f.Fetch(context.Background(), srv.URL+"/big.zip"); !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch for oversized package, got %v", err)
	}
}

func TestFetchDownloadsFromAllowedHost(t *testing.T) {
	payload := []byte("zip-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	data, ref, err := f.Fetch(context.Background(), srv.URL+"/pkg.zip")
	if err != nil || string(data) != string(payload) || ref != "" {
		t.Fatalf("got %q ref=%q err=%v", data, ref, err)
	}
}

func TestGitHubURLNormalization(t *testing.T) {
	f := &URLFetcher{Allowed: DefaultAllowedHosts()}
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	cands, _ := f.candidates(parse("https://github.com/anthropics/skills"))
	if len(cands) != 2 ||
		cands[0].url != "https://codeload.github.com/anthropics/skills/zip/refs/heads/main" ||
		cands[0].ref != "main" || cands[1].ref != "master" {
		t.Fatalf("repo URL candidates wrong: %+v", cands)
	}

	cands, _ = f.candidates(parse("https://github.com/anthropics/skills/tree/dev"))
	if len(cands) != 1 ||
		cands[0].url != "https://codeload.github.com/anthropics/skills/zip/refs/heads/dev" ||
		cands[0].ref != "dev" {
		t.Fatalf("tree URL candidate wrong: %+v", cands)
	}

	cands, _ = f.candidates(parse("https://codeload.github.com/o/r/zip/refs/heads/main"))
	if len(cands) != 1 || cands[0].url != "https://codeload.github.com/o/r/zip/refs/heads/main" {
		t.Fatalf("non-repo URL must pass through: %+v", cands)
	}
}
