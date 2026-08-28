// Grandchild fixture for the tree-reaping test. Proves it is alive by growing
// a file every 100ms, for as long as it lives. Its parent (reaper.mjs) does
// not wait for it and does not clean it up — the only thing that should ever
// stop it is the driver reaping the whole tree its parent was launched under.
import { appendFileSync, writeFileSync } from "node:fs";

const path = process.argv[2];
writeFileSync(path + ".pid", String(process.pid));
setInterval(() => {
  try {
    appendFileSync(path, Date.now() + "\n");
  } catch {
    // The directory can vanish once a test's TempDir is cleaned up; nothing
    // left to prove aliveness to at that point.
  }
}, 100);
