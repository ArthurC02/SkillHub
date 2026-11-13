from pathlib import Path
import shutil


root = Path("/tmp/caddy-tests")
source = root / "modules/caddyhttp/app.go"
text = source.read_text()
before = "\t\tstartedShutdown.Done()\n\n\t\t// possible if server failed to Start"
assert text.count(before) == 1
source.write_text(text.replace(before, "\t\tcaddy.FixtureRegisterListener(server.server)\n" + before))

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
