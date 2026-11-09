import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { PREFLIGHT } from "../../../testing/fixtures/platform";
import type { PreflightSummary } from "../lab.service";
import { PreflightFacts } from "./PreflightFacts";

const providerLine = (provider: object) =>
  renderToStaticMarkup(
    <PreflightFacts
      summary={{ ...PREFLIGHT.summary, provider } as unknown as PreflightSummary}
      cost={undefined}
      quota={undefined}
    />,
  );

test("an unassigned isolation strength reads 尚未定值, not a blank", () => {
  expect(providerLine({ name: "unassigned", rootless: false })).toContain(
    "unassigned（隔離:尚未定值）",
  );
});

test("isolation none is the real state 無隔離邊界", () => {
  expect(providerLine({ name: "clean", isolation_strength: "none", rootless: false })).toContain(
    "clean（隔離:無隔離邊界）",
  );
});
