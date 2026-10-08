package main

import (
	"strings"
	"testing"
)

const (
	hashX86 = "66a5b17354690ec2e4037b54080eeabde55a1503779ceb823d73856283a6836953e105493d7e38fb2dcf85e5c86e0f48c66a9abb0244a78bd612df938702475c"
	hashArm = "bac6a20f57b71c66538f9d33fb71e05c025bd1a420ed0c3afef58e0cde14273604a67411b886a1aae468de268dff66021ab7373f977a5715c6ab6f5ab62331fe"
)

func TestBaselineHashLinesAreRequiredForAReleaseBaseline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		file string
		want string
	}{
		{"both architectures pinned", "# c\nrelease-20260921.0\nsha512 x86_64 " + hashX86 + "\nsha512 aarch64 " + hashArm + "\n", ""},
		{"aarch64 line missing", "release-20260921.0\nsha512 x86_64 " + hashX86 + "\n", "no `sha512 aarch64"},
		{"x86_64 line missing", "release-20260921.0\nsha512 aarch64 " + hashArm + "\n", "no `sha512 x86_64"},
		{"hash one digit short", "release-20260921.0\nsha512 x86_64 " + hashX86[1:] + "\nsha512 aarch64 " + hashArm + "\n", "x86_64 is not 128"},
		{"hash one digit long", "release-20260921.0\nsha512 x86_64 " + hashX86 + "\nsha512 aarch64 " + hashArm + "0\n", "aarch64 is not 128"},
		{"hash not hexadecimal", "release-20260921.0\nsha512 x86_64 " + "g" + hashX86[1:] + "\nsha512 aarch64 " + hashArm + "\n", "x86_64 is not 128"},
		{"hash upper case", "release-20260921.0\nsha512 x86_64 " + strings.ToUpper(hashX86) + "\nsha512 aarch64 " + hashArm + "\n", "x86_64 is not 128"},
		{"comment line precedes a release with no hashes", "# note\nrelease-20260921.0\n", "no `sha512 x86_64"},
		{"unset baseline needs no hashes", "# c\nunset\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(gvisorHashProblems(tc.file), "\n")
			if tc.want == "" {
				if got != "" {
					t.Fatalf("expected no problems, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected a problem naming %q, got %q", tc.want, got)
			}
		})
	}
}

func daemonWithArgs(args string) []byte {
	return []byte(`{"runtimes":{"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":` + args + `}}}`)
}

func TestRunscRuntimeArgsStayWithinThePinnedSet(t *testing.T) {
	t.Parallel()
	pinned := `"--platform=systrap","--network=sandbox","--file-access=exclusive"`
	for _, tc := range []struct {
		name string
		json []byte
		want string
	}{
		{"the three pinned arguments", daemonWithArgs("[" + pinned + "]"), ""},
		{"empty list", daemonWithArgs("[]"), "missing"},
		{"runtimeArgs absent", []byte(`{"runtimes":{"runsc":{"path":"/x"}}}`), "missing"},
		{"debug", daemonWithArgs(`[` + pinned + `,"--debug"]`), `"--debug"`},
		{"debug-log", daemonWithArgs(`[` + pinned + `,"--debug-log=/tmp/x"]`), "--debug-log"},
		{"strace", daemonWithArgs(`[` + pinned + `,"--strace"]`), "--strace"},
		{"TESTONLY flag", daemonWithArgs(`[` + pinned + `,"--TESTONLY-unsafe-nonroot"]`), "--TESTONLY-unsafe-nonroot"},
		{"host-uds", daemonWithArgs(`[` + pinned + `,"--host-uds=all"]`), "--host-uds"},
		{"host-fifo", daemonWithArgs(`[` + pinned + `,"--host-fifo=open"]`), "--host-fifo"},
		{"network host", daemonWithArgs(`["--platform=systrap","--network=host","--file-access=exclusive"]`), "--network=host"},
		{"file-access shared", daemonWithArgs(`["--platform=systrap","--network=sandbox","--file-access=shared"]`), "--file-access=shared"},
		{"rootless", daemonWithArgs(`[` + pinned + `,"--rootless"]`), "--rootless"},
		{"ignore-cgroups", daemonWithArgs(`[` + pinned + `,"--ignore-cgroups"]`), "--ignore-cgroups"},
		{"a pinned argument dropped", daemonWithArgs(`["--platform=systrap","--network=sandbox"]`), `missing "--file-access=exclusive"`},
		{"not json", []byte(`{`), "unexpected end of JSON input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := strings.Join(gvisorRuntimeArgProblems(tc.json), "\n")
			if tc.want == "" {
				if got != "" {
					t.Fatalf("expected no problems, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected a problem naming %q, got %q", tc.want, got)
			}
		})
	}
}
