from pathlib import Path
import shutil


root = Path("/tmp/caddy-tests")
source = root / "caddytest/integration/h2listener_test.go"
text = source.read_text()
before = "\tlocalhost {\n\t\trespond "
assert text.count(before) == 1
source.write_text(
    text.replace(
        before, "\tlocalhost {\n\t\ttls /tmp/sni-certificate.pem /tmp/sni-key.pem\n\t\trespond "
    )
)

source = root / "modules/logging/filewriter_test.go"
text = source.read_text()
before = "\tfiles, err := os.ReadDir(dir)"
assert text.count(before) == 1
text = text.replace(
    before,
    "\tif err := logger.Close(); err != nil {\n"
    '\t\tt.Fatalf("failed to close log writer: %v", err)\n\t}\n\n' + before,
)
before = """\t// We might get 2 or 3 files depending
\t// on the race between compressed log file generation,
\t// removal of the non compressed file and reading the directory.
\t// Ordering of the files are [ test-*.log test-*.log.gz test.log ]
\tif len(files) < 2 || len(files) > 3 {"""
assert text.count(before) == 1
text = text.replace(before, "\tif len(files) != 2 {")
before = 'wantPattern := "test-*-*-*-*-*.*.log"'
assert text.count(before) == 1
source.write_text(text.replace(before, 'wantPattern := "test-*-*-*-*-*.*.log.gz"'))

source = root / "modules/caddyhttp/app.go"
text = source.read_text()
before = "\t\tstartedShutdown.Done()\n\n\t\t// possible if server failed to Start"
assert text.count(before) == 1
source.write_text(
    text.replace(before, "\t\tcaddy.FixtureRegisterListener(server.server)\n" + before)
)

source = root / "caddytest/caddytest.go"
text = source.read_text()
assert '"github.com/caddyserver/caddy/v2"' not in text
assert text.count("import (") == 1
text = text.replace("import (", 'import (\n\t"github.com/caddyserver/caddy/v2"', 1)
assert text.count("func (tc *Tester) InitServer(") == 1
start = text.index("func (tc *Tester) InitServer(")
end = text.index("\n}\n", start)
source.write_text(text[:end] + "\n\tcaddy.FixtureWaitListeners()" + text[end:])

fixture = root / "listener_fixture.go"
assert not fixture.exists()
shutil.copyfile(Path(__file__).with_name("listener_fixture.go"), fixture)
