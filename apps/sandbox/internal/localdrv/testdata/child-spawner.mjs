import { spawn } from "node:child_process";

const count = Number(process.env.SKILLHUB_USER_PROMPT);
for (let i = 0; i < count; i++) {
  try {
    spawn("ping", ["-n", "30", "127.0.0.1"], { stdio: "ignore" }).on("error", () => {});
  } catch {}
}

setTimeout(() => process.exit(0), 2000);
