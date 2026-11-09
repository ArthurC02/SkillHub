import { renderToStaticMarkup } from "react-dom/server";
import { expect, test, vi } from "vitest";
import { ARTIFACT_ROW } from "../../../testing/fixtures/platform";
import type { DownloadArtifact } from "../packaging.service";
import { DownloadArtifactFacts } from "./DownloadArtifactFacts";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children?: unknown }) => children,
}));

const profileLine = (artifact: object) =>
  renderToStaticMarkup(
    <DownloadArtifactFacts
      artifact={{ ...ARTIFACT_ROW, ...artifact } as unknown as DownloadArtifact}
    />,
  );

test.each([
  [
    "a standard package, which has no profile",
    { target: "standard" },
    "Profile 版本：不適用（標準套件沒有 Profile）",
  ],
  [
    "a profiled package whose version was not reported",
    { target: "claude-code" },
    "Profile 版本：未測量",
  ],
  [
    "a profiled package with its version",
    { target: "claude-code", profile_version: "2.1" },
    "Profile 版本：2.1",
  ],
] as const)("%s", (_name, artifact, want) => {
  expect(profileLine(artifact)).toContain(want);
});
