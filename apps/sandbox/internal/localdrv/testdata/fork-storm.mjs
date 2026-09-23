import { spawn } from "node:child_process";

const attempts = 200;
let live = 0;
let refused = 0;

for (let i = 0; i < attempts; i++) {
  const child = spawn("sleep", ["30"], { stdio: "ignore" });
  child.on("spawn", () => live++);
  child.on("error", () => refused++);
}

setTimeout(() => {
  console.log(`the storm: live=${live} refused=${refused} attempts=${attempts}`);
  process.exit(0);
}, 2000);
