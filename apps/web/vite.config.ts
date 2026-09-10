import { execSync } from "node:child_process";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

function buildId(): string {
  if (process.env.GITHUB_SHA) return process.env.GITHUB_SHA.slice(0, 12);
  try {
    const sha = execSync("git rev-parse --short=12 HEAD", { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
    return `${sha}（本機建置，未經 CI）`;
  } catch {
    return "未知（建置環境沒有 git 也沒有 GITHUB_SHA）";
  }
}

export default defineConfig({
  plugins: [react()],
  define: { "import.meta.env.VITE_BUILD_ID": JSON.stringify(buildId()) },
  test: {
    environment: "jsdom",
    // e2e/** is Playwright's; vitest's default glob would otherwise collect
    // those specs too and fail at import time under the wrong runner.
    exclude: ["**/node_modules/**", "**/dist/**", "e2e/**"],
  },
});
