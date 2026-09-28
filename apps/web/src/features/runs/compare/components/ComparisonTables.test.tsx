import { StrictMode, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import { COMPARISON, comparisonSide } from "../../../../testing/fixtures/platform";
import type { ComparisonSide, RunComparison } from "../../evaluation.service";
import { CriterionMatrixTable } from "./CriterionMatrixTable";
import { RunStatusTable } from "./RunStatusTable";

const COMPARISON_SIDES = COMPARISON.runs as unknown as ComparisonSide[];
const CRITERION_MATRIX =
  COMPARISON.criterion_matrix as unknown as RunComparison["criterion_matrix"];

let container: HTMLDivElement;
let root: Root;

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

test("CriterionMatrixTable says there is nothing to compare when the matrix is empty", async () => {
  await mount(<CriterionMatrixTable criterionMatrix={[]} sides={COMPARISON_SIDES} />);

  expect(text()).toContain("沒有可對照的驗收條件。");
});

test("CriterionMatrixTable lists each criterion row with its per-side result", async () => {
  await mount(<CriterionMatrixTable criterionMatrix={CRITERION_MATRIX} sides={COMPARISON_SIDES} />);

  expect(text()).toContain("輸出的 CSV 含有 email 欄位");
  expect(text()).toContain("未評估");
});
