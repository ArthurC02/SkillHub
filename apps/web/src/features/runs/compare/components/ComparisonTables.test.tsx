import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { COMPARISON, comparisonSide } from "../../../../testing/fixtures/platform";
import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { CriterionMatrixTable } from "./CriterionMatrixTable";
import { ComparisonTables } from "./ComparisonTables";
import { RunStatusTable } from "./RunStatusTable";

const COMPARISON_SIDES = COMPARISON.runs as unknown as ComparisonSide[];
const CRITERION_MATRIX =
  COMPARISON.criterion_matrix as unknown as RunComparison["criterion_matrix"];

let container: HTMLDivElement;
let root: Root;

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

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
});

afterEach(async () => {
  await act(async () => root?.unmount());
  container.remove();
});

async function mount(node: React.ReactElement) {
  root = createRoot(container);
  await act(async () => {
    root.render(<StrictMode>{node}</StrictMode>);
  });
}

const text = () => container.textContent ?? "";

function sideWithoutRerunLink(runId: string, evaluated: boolean): ComparisonSide {
  return {
    ...comparisonSide(runId, evaluated),
    inputs_available: false,
  } as unknown as ComparisonSide;
}

test("RunStatusTable prints one shared cost note when both sides cite the same source", async () => {
  const sides = [sideWithoutRerunLink("run-a", true), sideWithoutRerunLink("run-b", false)];
  await mount(<RunStatusTable sides={sides} />);

  const notes = [...container.querySelectorAll(".note")].filter((el) =>
    el.textContent?.includes("模型閘道對這個 Run 的 per-key 實付"),
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

  const notes = [...container.querySelectorAll(".note")].filter((el) =>
    el.textContent?.includes("權威來源"),
  );
  expect(notes).toHaveLength(2);
});

test("RunStatusTable keeps each side connected to its own Run and immutable version", async () => {
  const sides = [
    { ...sideWithoutRerunLink("run-a", true), skill_id: "skill-a", skill_version_id: "version-a" },
    { ...sideWithoutRerunLink("run-b", false), skill_id: "skill-b", skill_version_id: "version-b" },
  ];
  await mount(<RunStatusTable sides={sides} />);

  expect(
    Array.from(container.querySelectorAll("a")).map((link) => link.getAttribute("href")),
  ).toEqual([
    "/runs/run-a",
    "/runs/run-b",
    "/skills/skill-a/versions/version-a",
    "/skills/skill-b/versions/version-b",
  ]);
});

test("RunStatusTable distinguishes an absent output from an error-free run", async () => {
  const sides = [sideWithoutRerunLink("run-a", true), sideWithoutRerunLink("run-b", false)].map(
    (side) => ({ ...side, final_output: undefined, errors: [] }),
  );
  await mount(<RunStatusTable sides={sides} />);

  const rows = Array.from(container.querySelectorAll("tbody tr"));
  expect(rows.find((row) => row.textContent?.includes("最終輸出"))?.textContent).toContain(
    "未產生",
  );
  expect(rows.find((row) => row.textContent?.includes("錯誤"))?.textContent).toContain(
    "沒有錯誤紀錄",
  );
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
    container.querySelectorAll<HTMLElement>('.table-scroll[role="region"]'),
  );
  expect(regions).toHaveLength(2);
  expect(regions.every((region) => region.getAttribute("aria-label")?.includes("左右捲動"))).toBe(
    true,
  );
  expect(container.querySelectorAll(".table-scroll-hint")).toHaveLength(1);
});
