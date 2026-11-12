import { expect, test, vi } from "vitest";
import { COMPARISON, comparisonSide } from "../../../../testing/fixtures/platform";
import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { CriterionMatrixTable } from "./CriterionMatrixTable";
import { ComparisonTables } from "./ComparisonTables";
import { RunStatusTable } from "./RunStatusTable";
import { mountedComponent } from "../../../../testing/component";

const COMPARISON_SIDES = COMPARISON.runs as unknown as ComparisonSide[];
const CRITERION_MATRIX =
  COMPARISON.criterion_matrix as unknown as RunComparison["criterion_matrix"];

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => {
    const href = Object.entries(params ?? {}).reduce(
      (path, [key, value]) => path.replace(`$${key}`, value),
      to,
    );
    return <a href={href}>{children as never}</a>;
  },
}));

const { container, mount, text } = mountedComponent();

function sideWithoutRerunLink(runId: string, evaluated: boolean): ComparisonSide {
  return {
    ...comparisonSide(runId, evaluated),
    inputs_available: false,
  } as unknown as ComparisonSide;
}

test("RunStatusTable prints one shared cost note when both sides cite the same source", async () => {
  const sides = [sideWithoutRerunLink("run-a", true), sideWithoutRerunLink("run-b", false)];
  await mount(<RunStatusTable sides={sides} />);

  const notes = [...container().querySelectorAll(".note")].filter((el) =>
    el.textContent?.includes("模型閘道對這次試跑的 per-key 實付"),
  );
  expect(notes).toHaveLength(1);
});

test("RunStatusTable prints a note per side when the sides cite different sources", async () => {
  const sides = [
    sideWithoutRerunLink("run-a", true),
    {
      ...sideWithoutRerunLink("run-b", false),
      cost: { ...comparisonSide("run-b", false).cost, authoritative_source: "另一個來源" },
    },
  ];
  await mount(<RunStatusTable sides={sides} />);

  const notes = [...container().querySelectorAll(".note")].filter((el) =>
    el.textContent?.includes("權威來源"),
  );
  expect(notes).toHaveLength(2);
});

test("RunStatusTable keeps each side connected to its own 試跑紀錄 and immutable version", async () => {
  const sides = [
    { ...sideWithoutRerunLink("run-a", true), skill_id: "skill-a", skill_version_id: "version-a" },
    { ...sideWithoutRerunLink("run-b", false), skill_id: "skill-b", skill_version_id: "version-b" },
  ];
  await mount(<RunStatusTable sides={sides} />);

  expect(
    Array.from(container().querySelectorAll("a")).map((link) => link.getAttribute("href")),
  ).toEqual([
    "/runs/run-a",
    "/runs/run-b",
    "/skills/skill-a/versions/version-a",
    "/skills/skill-b/versions/version-b",
  ]);
});

test("RunStatusTable tells a field the server did not report from a measured zero", async () => {
  const sides = [
    { ...sideWithoutRerunLink("run-a", true), final_output: undefined, errors: undefined },
    { ...sideWithoutRerunLink("run-b", false), final_output: "", errors: [] },
  ];
  await mount(<RunStatusTable sides={sides} />);

  const cells = (header: string) =>
    Array.from(
      Array.from(container().querySelectorAll("tbody tr"))
        .find((row) => row.querySelector("th")?.textContent === header)
        ?.querySelectorAll("td") ?? [],
      (cell) => cell.textContent,
    );
  expect(cells("最終輸出")).toEqual(["未測量", "0 字"]);
  expect(cells("錯誤")).toEqual(["未測量", "0 筆"]);
});

test("RunStatusTable keeps untrusted provider wording behind technical details", async () => {
  const sides = [
    {
      ...sideWithoutRerunLink("run-a", true),
      errors: [{ category: "provision", code: "provider_error", message: "no slot" }],
    },
    {
      ...sideWithoutRerunLink("run-b", false),
      errors: [{ message: "opaque provider response" }],
    },
  ];
  await mount(<RunStatusTable sides={sides} />);

  const errorRow = Array.from(container().querySelectorAll("tbody tr")).find((row) =>
    row.querySelector("th")?.textContent?.includes("錯誤"),
  );
  const disclosures = Array.from(errorRow?.querySelectorAll("details") ?? []);
  expect(disclosures).toHaveLength(2);
  expect(disclosures.every((item) => !item.hasAttribute("open"))).toBe(true);
  expect(disclosures[0]?.textContent).toContain("provision/provider_error");
  expect(disclosures[0]?.textContent).toContain("no slot");
  expect(disclosures[1]?.textContent).toContain("錯誤代碼未測量");
  expect(errorRow?.textContent).toContain("這次試跑留有錯誤紀錄");
});

test("CriterionMatrixTable says there is nothing to compare when the matrix is empty", async () => {
  await mount(<CriterionMatrixTable criterionMatrix={[]} sides={COMPARISON_SIDES} />);

  expect(text()).toContain("沒有可對照的驗收條件。");
});

test("CriterionMatrixTable lists each criterion row with its per-side result", async () => {
  await mount(<CriterionMatrixTable criterionMatrix={CRITERION_MATRIX} sides={COMPARISON_SIDES} />);

  expect(text()).toContain("輸出的 CSV 含有 email 欄位");
  expect(text()).toContain("未評估");
});

test("comparison tables name each scroll region and expose a mobile scroll hint", async () => {
  const runs = [sideWithoutRerunLink("run-a", true), sideWithoutRerunLink("run-b", false)];
  await mount(
    <ComparisonTables
      data={{ ...(COMPARISON as unknown as RunComparison), runs, version_diff_url: undefined }}
    />,
  );

  const regions = Array.from(
    container().querySelectorAll<HTMLElement>('.table-scroll[role="region"]'),
  );
  expect(regions).toHaveLength(2);
  expect(regions.every((region) => region.getAttribute("aria-label")?.includes("左右捲動"))).toBe(
    true,
  );
  expect(container().querySelectorAll(".table-scroll-hint")).toHaveLength(1);
});
