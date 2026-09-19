package ingest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fakeFetcher struct {
	policy  sourcePolicy
	sources map[string][]byte
}

func (f *fakeFetcher) Normalize(rawURL string) (string, error) { return f.policy.normalize(rawURL) }

func (f *fakeFetcher) Fetch(_ context.Context, rawURL string) ([]byte, string, error) {
	normalized, err := f.Normalize(rawURL)
	if err != nil {
		return nil, "", err
	}
	data, held := f.sources[normalized]
	if !held {
		return nil, "", fmt.Errorf("%w: 來源不存在。", ErrFetch)
	}
	return data, "", nil
}

func (f *fakeFetcher) Probe(_ context.Context, rawURL string) error {
	normalized, err := f.Normalize(rawURL)
	if err != nil {
		return err
	}
	if _, held := f.sources[normalized]; !held {
		return fmt.Errorf("%w: 來源不存在。", ErrFetch)
	}
	return nil
}

type fetcherUnderTest struct {
	fetcher SourceFetcher
	present string
	absent  string
}

func fetchersRefusingInsecure(t *testing.T) map[string]SourceFetcher {
	t.Helper()
	allowed := map[string]bool{"allowed.test": true}
	return map[string]SourceFetcher{
		"fake":         &fakeFetcher{policy: sourcePolicy{allowedHosts: allowed}},
		"http adapter": FetcherOrNone(&URLFetcher{Allowed: allowed}),
	}
}

func fetchersWithAReadableSource(t *testing.T, body []byte) map[string]fetcherUnderTest {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/skill.zip" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	host := mustHost(t, srv.URL)

	return map[string]fetcherUnderTest{
		"fake": {
			fetcher: &fakeFetcher{
				policy:  sourcePolicy{allowedHosts: map[string]bool{"allowed.test": true}, allowInsecure: true},
				sources: map[string][]byte{"http://allowed.test/skill.zip": body},
			},
			present: "http://ALLOWED.test/skill.zip",
			absent:  "http://allowed.test/missing.zip",
		},
		"http adapter": {
			fetcher: FetcherOrNone(&URLFetcher{
				Allowed: map[string]bool{host: true}, AllowInsecure: true,
			}),
			present: strings.Replace(srv.URL, host, strings.ToUpper(host), 1) + "/skill.zip",
			absent:  srv.URL + "/missing.zip",
		},
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing the test server's URL: %v", err)
	}
	return u.Host
}

func TestEveryFetcherRefusesTheSameSources(t *testing.T) {
	for name, fetcher := range fetchersRefusingInsecure(t) {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name, rawURL string
			}{
				{"a host the allow list does not name", "https://elsewhere.test/skill.zip"},
				{"plain HTTP when only HTTPS is allowed", "http://allowed.test/skill.zip"},
				{"a URL carrying credentials", "https://user:pw@allowed.test/skill.zip"},
				{"a URL carrying a query string", "https://allowed.test/skill.zip?token=x"},
				{"a URL carrying a fragment", "https://allowed.test/skill.zip#part"},
				{"text that is not a URL", "://"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					refusal := errorText(fetcher.Normalize(tc.rawURL))
					if refusal == "" {
						t.Fatalf("Normalize(%q) allowed a source the policy forbids", tc.rawURL)
					}
					data, _, err := fetcher.Fetch(context.Background(), tc.rawURL)
					if !errors.Is(err, ErrFetch) {
						t.Errorf("Fetch(%q) = %v, want a refusal the domain can recognise", tc.rawURL, err)
					}
					if got := errText(err); got != refusal {
						t.Errorf("Fetch(%q) refused with %q but Normalize refused with %q; the fetch "+
							"reached past the policy and failed for some other reason", tc.rawURL, got, refusal)
					}
					if len(data) != 0 {
						t.Errorf("Fetch(%q) returned %d bytes from a refused source", tc.rawURL, len(data))
					}
					probeErr := fetcher.Probe(context.Background(), tc.rawURL)
					if !errors.Is(probeErr, ErrFetch) {
						t.Errorf("Probe(%q) = %v, want a refusal the domain can recognise", tc.rawURL, probeErr)
					}
					if got := errText(probeErr); got != refusal {
						t.Errorf("Probe(%q) refused with %q but Normalize refused with %q; the probe "+
							"reached past the policy", tc.rawURL, got, refusal)
					}
				})
			}
		})
	}
}

func TestEveryFetcherReadsAnAllowedSourceAndInventsNoProvenance(t *testing.T) {
	body := []byte("PK\x03\x04 a package")
	for name, under := range fetchersWithAReadableSource(t, body) {
		t.Run(name, func(t *testing.T) {
			data, ref, err := under.fetcher.Fetch(context.Background(), under.present)
			if err != nil {
				t.Fatalf("fetching an allowed source: %v", err)
			}
			if string(data) != string(body) {
				t.Errorf("fetched %q, want %q", data, body)
			}
			if ref != "" {
				t.Errorf("fetch named the git ref %q for a source that carries none; an invented ref "+
					"would be stored as this version's provenance", ref)
			}
			if err := under.fetcher.Probe(context.Background(), under.present); err != nil {
				t.Errorf("Probe on a source Fetch just read = %v, want it reachable", err)
			}
		})
	}
}

func TestEveryFetcherCanonicalisesASourceTheSameWayTwice(t *testing.T) {
	for name, under := range fetchersWithAReadableSource(t, []byte("x")) {
		t.Run(name, func(t *testing.T) {
			once, err := under.fetcher.Normalize(under.present)
			if err != nil {
				t.Fatalf("normalising: %v", err)
			}
			if host := mustHost(t, once); host != strings.ToLower(host) {
				t.Errorf("normalised %q keeps the host's original case; the same source spelled two "+
					"ways would be stored twice", once)
			}
			twice, err := under.fetcher.Normalize(once)
			if err != nil {
				t.Fatalf("normalising the normalised form: %v", err)
			}
			if twice != once {
				t.Errorf("normalising twice gave %q then %q; the canonical form is not a fixed point",
					once, twice)
			}
		})
	}
}

func TestEveryFetcherReportsAnAbsentSourceWithoutBytes(t *testing.T) {
	for name, under := range fetchersWithAReadableSource(t, []byte("x")) {
		t.Run(name, func(t *testing.T) {
			data, _, err := under.fetcher.Fetch(context.Background(), under.absent)
			if err == nil {
				t.Fatal("fetching a source that is not there succeeded")
			}
			if len(data) != 0 {
				t.Errorf("a failed fetch still returned %d bytes", len(data))
			}
			if err := under.fetcher.Probe(context.Background(), under.absent); err == nil {
				t.Error("probing a source that is not there reported it reachable")
			}
		})
	}
}

func errorText(_ string, err error) string { return errText(err) }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
