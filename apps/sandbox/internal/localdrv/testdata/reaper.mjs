// Test fixture for 02:PORT-010's acceptance criterion: reaping must cover the
// whole process tree, not just the process the driver spawned directly. This
// stands in for run.mjs — it spawns one grandchild and then stays alive
// itself, so a test can kill *this* process through the driver and check
// whether the grandchild survived it, the same shape
// docs/plans/mvp/m6/report-local-driver.md §2 measured.
//
// `detached: true` here is load-bearing, not incidental, and was arrived at
// the hard way (by a test that stayed green through a mutated,
// parent-only-kill terminate() before this flag was added — see the report
// to whoever picks this driver up next for the full trail). An *ordinary*
// (non-detached) child on Windows turned out to already survive a plain
// `taskkill /PID <parent>` without this driver's help: Node/libuv puts a
// non-detached child in a job object of Node's *own*, and that job's handle
// closing when the parent process dies takes the child with it — measured
// directly with PowerShell's Start-Process/taskkill, no Go and no
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE of this driver's own involved at all.
// `detached: true` is specifically how a child opts *out* of that protection,
// which is what makes this fixture actually exercise this driver's own
// reaping rather than Node's. See tree_windows.go and tree_unix.go for what
// each platform's implementation catches here and what it still does not.
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const outDir = process.env.SKILLHUB_OUTDIR;
const here = dirname(fileURLToPath(import.meta.url));

const child = spawn(process.execPath, [join(here, "heartbeat.mjs"), join(outDir, "heartbeat.log")], {
  stdio: "ignore",
  detached: true,
});
child.unref();

// Keep this process (and the event loop) alive until the driver kills it.
setInterval(() => {}, 1000);
