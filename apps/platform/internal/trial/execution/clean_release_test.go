package run

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releasedVersion = "22222222-2222-2222-2222-222222222222"

func writeReleases(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "releases.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnOperatorReleaseRunsExactlyTheVersionItNames(t *testing.T) {
	other := "33333333-3333-3333-3333-333333333333"

	for _, tc := range []struct {
		what     string
		file     string
		unset    bool
		wantPass bool
		wantSaid []string
	}{
		{
			what:     "an id and a reason",
			file:     "# 台上要跑觀眾帶來的 skill\n" + releasedVersion + " demo 2026-09-02, content reviewed by me\n",
			wantPass: true,
		},
		{
			what:     "a reason with several words keeps all of them",
			file:     releasedVersion + "   審過了，只跑這一版\n",
			wantPass: true,
		},

		{
			what:     "an id with no reason after it",
			file:     releasedVersion + "\n",
			wantSaid: []string{"a line with no reason after the id is not a release"},
		},
		{
			what:     "an id with only whitespace after it",
			file:     releasedVersion + "    \n",
			wantSaid: []string{"not a release"},
		},

		{
			what:     "a different version released",
			file:     other + " somebody else's skill\n",
			wantSaid: []string{releasedVersion},
		},
		{
			what:     "the release commented out",
			file:     "#" + releasedVersion + " I changed my mind\n",
			wantSaid: []string{"needs a deployment with a real sandbox"},
		},
		{
			what:     "the id as a prefix of a longer id",
			file:     releasedVersion + "x still not this one\n",
			wantSaid: []string{"only runs curated material"},
		},
		{
			what:     "a file that is not there",
			file:     "",
			wantSaid: []string{releasedVersion},
		},

		{
			what:     "the variable never set",
			unset:    true,
			wantSaid: []string{"has not set that variable", releasedVersion},
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Setenv("SKILLHUB_CLEAN_MODE", "1")
			switch {
			case tc.unset:
				t.Setenv(cleanModeReleaseFile, "")
			case tc.file == "":
				t.Setenv(cleanModeReleaseFile, filepath.Join(t.TempDir(), "absent.txt"))
			default:
				t.Setenv(cleanModeReleaseFile, writeReleases(t, tc.file))
			}

			svc := &Service{ReadContentSource: stubContentSource(ContentSource{CurationTier: "indexed"}, true, nil)}
			err := svc.requireCuratedContent(t.Context(), contentSourceRun())
			if tc.wantPass {
				if err != nil {
					t.Fatalf("a released version was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("content nobody released ran on a driver with no isolation boundary")
			}
			if !errors.Is(err, ErrContentNotCurated) {
				t.Errorf("error = %v, want it to wrap ErrContentNotCurated", err)
			}
			for _, want := range tc.wantSaid {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("reason = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestUsingAReleaseSaysSoWithTheReasonTheOperatorGave(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	t.Setenv(cleanModeReleaseFile, writeReleases(t, releasedVersion+" reviewed for the 09-02 demo\n"))

	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	svc := &Service{ReadContentSource: stubContentSource(ContentSource{CurationTier: "indexed"}, true, nil)}
	if err := svc.requireCuratedContent(t.Context(), contentSourceRun()); err != nil {
		t.Fatalf("a released version was refused: %v", err)
	}

	for _, want := range []string{
		"reviewed for the 09-02 demo",
		releasedVersion,
		"no isolation boundary",
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log = %q, want it to mention %q", logged.String(), want)
		}
	}
}

func TestTheLauncherFillsInTheVariableThisPackageReads(t *testing.T) {
	const launcher = "../../../../../tools/cleanmode/start.mjs"
	src, err := os.ReadFile(launcher)
	if err != nil {
		t.Fatalf("read %s: %v", launcher, err)
	}
	if !strings.Contains(string(src), cleanModeReleaseFile+": RELEASES_FILE") {
		t.Errorf("%s does not set %s; the clean mode would have no release switch at all "+
			"while every refusal still tells the operator to use one", launcher, cleanModeReleaseFile)
	}
}

func TestTheReleaseListIsNeverEvenReadOutsideTheCleanTestMode(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "")
	t.Setenv("DEV_LOGIN", "1")

	t.Setenv(cleanModeReleaseFile, t.TempDir())

	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	if reason, released := operatorReleased(releasedVersion); released {
		t.Fatalf("a version was released off an unreadable list, reason %q", reason)
	}
	if logged.Len() == 0 {
		t.Fatal("an unreadable release list said nothing; a typo'd path is indistinguishable from a broken switch")
	}

	logged.Reset()
	if err := (&Service{}).requireCuratedContent(t.Context(), contentSourceRun()); err != nil {
		t.Fatalf("a production deployment was refused: %v", err)
	}
	if logged.Len() != 0 {
		t.Errorf("the release list was consulted outside the clean test mode: %q", logged.String())
	}
}

func TestTheReleaseSurvivesTheWayPeopleActuallyTypeIt(t *testing.T) {
	for _, tc := range []struct {
		what string
		line string
		why  string
	}{
		{"a tab between the id and the reason", releasedVersion + "\tdemo", "對齊兩欄時手會去按 Tab"},
		{"CRLF line endings", releasedVersion + " demo\r\n", "這個模式的目標機器是 Windows"},
		{"a UTF-8 BOM at the top of the file", "\ufeff" + releasedVersion + " demo", "記事本新建檔案時就是這樣存的"},
		{"backticks pasted along with the id", "`" + releasedVersion + "` demo", "拒絕訊息把 id 放在反引號裡"},
		{"a list marker in front", "- " + releasedVersion + " demo", "貼進檔案的時候順手當成清單"},
		{"the id in upper case", strings.ToUpper(releasedVersion) + " demo", "UUID 依 RFC 4122 不分大小寫"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Setenv("SKILLHUB_CLEAN_MODE", "1")
			t.Setenv(cleanModeReleaseFile, writeReleases(t, tc.line))
			svc := &Service{ReadContentSource: stubContentSource(ContentSource{CurationTier: "indexed"}, true, nil)}
			if err := svc.requireCuratedContent(t.Context(), contentSourceRun()); err != nil {
				t.Fatalf("a release written this way was ignored (%s): %v", tc.why, err)
			}
		})
	}
}

func TestALineThatLooksLikeAReleaseAndIsNotSaysSo(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	t.Setenv(cleanModeReleaseFile, writeReleases(t, "release "+releasedVersion+" for the demo\n"))

	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	if _, released := operatorReleased(releasedVersion); released {
		t.Fatal("a line whose first token is not the version id released it anyway")
	}
	if !strings.Contains(logged.String(), "does not release it") {
		t.Errorf("log = %q, want it to say the line was seen and did not count", logged.String())
	}
}
