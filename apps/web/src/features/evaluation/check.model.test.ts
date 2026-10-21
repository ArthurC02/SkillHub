import { describe, expect, it } from "vitest";
import {
  appendChild,
  emptyNode,
  forRequest,
  placeNodes,
  problems,
  removeAt,
  replaceAt,
  type CheckKind,
  type CheckNode,
} from "./check.model";

const kind = (k: Partial<CheckKind> & { kind: string }): CheckKind => ({
  category: "grader",
  label: k.kind,
  description: "",
  values: { min: 0, max: 0 },
  children: { min: 0, max: 0 },
  at_least: false,
  ...k,
});

const KINDS = [
  kind({ kind: "contains_values", values: { min: 1, max: 20 } }),
  kind({ kind: "judge" }),
  kind({ kind: "sequence", category: "composite", children: { min: 1, max: 50 } }),
  kind({ kind: "if", category: "composite", children: { min: 2, max: 3 } }),
  kind({
    kind: "each",
    category: "composite",
    values: { min: 1, max: 20 },
    children: { min: 1, max: 1 },
    at_least: true,
  }),
];
const LIMITS = { maxNodes: 50, maxDepth: 8 };

for (const character of ["a", "😀"]) {
  for (const count of [199, 200, 201]) {
    it(`validates ${character} check values at ${count} code points`, () => {
      const tree: CheckNode = { kind: "contains_values", values: [character.repeat(count)] };
      expect(problems(tree, KINDS, LIMITS).get("0") ?? []).toEqual(
        count === 201 ? ["值最多 200 個字，目前 201 個字。"] : [],
      );
    });
  }
  it(`validates trimmed ${character} values at the wire limit inside a child`, () => {
    const value = ` ${character.repeat(200)} `;
    const tree: CheckNode = {
      kind: "sequence",
      children: [{ kind: "contains_values", values: [value] }],
    };
    expect(problems(tree, KINDS, LIMITS).size).toBe(0);
    expect(forRequest(tree)).toEqual({
      kind: "sequence",
      children: [{ kind: "contains_values", values: [character.repeat(200)] }],
    });
    expect(tree.children![0].values).toEqual([value]);
  });
}

for (const { name, value, expected } of [
  { name: "zero", value: 0, expected: [] },
  { name: "item count", value: 1, expected: [] },
  { name: "fraction", value: 0.5, expected: ["至少通過數必須是整數"] },
  { name: "not a number", value: Number.NaN, expected: ["至少通過數必須是整數"] },
]) {
  it(`validates the ${name} fan-out threshold against the integer contract`, () => {
    const tree: CheckNode = {
      kind: "each",
      values: ["資料"],
      children: [{ kind: "judge" }],
      at_least: value,
    };
    expect(problems(tree, KINDS, LIMITS).get("0") ?? []).toEqual(expected);
  });
}

const gated: CheckNode = {
  kind: "sequence",
  children: [{ kind: "contains_values", values: ["17"] }, { kind: "judge" }],
};

describe("check tree edits", () => {
  it("replaces, appends and removes a node at a path without touching the original", () => {
    const replaced = replaceAt(gated, [1], { kind: "contains_values", values: ["18"] });
    expect(replaced.children![1]).toEqual({ kind: "contains_values", values: ["18"] });
    expect(gated.children![1]).toEqual({ kind: "judge" });

    const appended = appendChild(gated, [], { kind: "judge" });
    expect(appended.children).toHaveLength(3);

    expect(removeAt(gated, [0])!.children).toEqual([{ kind: "judge" }]);
    expect(removeAt(gated, [])).toBeNull();
  });

  it("starts a new node with as many blank values as its kind requires and no children for a grader", () => {
    expect(emptyNode(KINDS[0])).toEqual({ kind: "contains_values", values: [""] });
    expect(emptyNode(KINDS[1])).toEqual({ kind: "judge" });
    expect(emptyNode(KINDS[2])).toEqual({ kind: "sequence", children: [] });
  });
});

describe("problems before saving", () => {
  it("names each node that breaks its kind's shape, by path", () => {
    const tree: CheckNode = {
      kind: "sequence",
      children: [
        { kind: "contains_values", values: [" "] },
        { kind: "if", children: [{ kind: "judge" }] },
        { kind: "regex" },
        { kind: "each", values: ["a", "b"], at_least: 3, children: [{ kind: "judge" }] },
      ],
    };
    const found = problems(tree, KINDS, LIMITS);
    expect(found.get("0.0")).toEqual(["有空白的值"]);
    expect(found.get("0.1")).toEqual(["要 2 到 3 個子節點，現在 1 個"]);
    expect(found.get("0.2")).toEqual(["平台不認得「regex」這種節點"]);
    expect(found.get("0.3")).toEqual(["至少通過數要在 0 到 2 之間"]);
    expect(found.has("0")).toBe(false);
  });

  it("accepts a tree at the node and depth limits and refuses one past each", () => {
    const wide = (n: number): CheckNode => ({
      kind: "sequence",
      children: Array.from({ length: n - 1 }, () => ({ kind: "judge" })),
    });
    const deep = (d: number): CheckNode =>
      d === 1 ? { kind: "judge" } : { kind: "sequence", children: [deep(d - 1)] };
    expect(problems(wide(50), KINDS, LIMITS).has("tree")).toBe(false);
    expect(problems(wide(51), KINDS, LIMITS).get("tree")).toEqual(["一個檢查最多 50 個節點"]);
    expect(problems(deep(8), KINDS, LIMITS).has("tree")).toBe(false);
    expect(problems(deep(9), KINDS, LIMITS).get("tree")).toEqual(["一個檢查最多 8 層"]);
  });

  it("finds nothing wrong with a well-formed tree", () => {
    expect(problems(gated, KINDS, LIMITS).size).toBe(0);
  });
});

describe("what is sent", () => {
  it("trims values and leaves out empty lists and a zero at_least", () => {
    expect(
      forRequest({
        kind: "each",
        values: [" a "],
        at_least: 0,
        children: [{ kind: "judge", values: [], children: [] }],
      }),
    ).toEqual({ kind: "each", values: ["a"], children: [{ kind: "judge" }] });
  });
});

describe("placing nodes on the canvas", () => {
  it("puts each node one column right of its parent and centres a parent on its children", () => {
    const tree: CheckNode = {
      kind: "sequence",
      children: [
        { kind: "judge" },
        { kind: "if", children: [{ kind: "judge" }, { kind: "judge" }] },
      ],
    };
    const at = Object.fromEntries(placeNodes(tree).map((p) => [p.path.join("."), p]));
    expect(at[""]).toMatchObject({ column: 0, row: 0.75 });
    expect(at["0"]).toMatchObject({ column: 1, row: 0 });
    expect(at["1"]).toMatchObject({ column: 1, row: 1.5 });
    expect(at["1.0"]).toMatchObject({ column: 2, row: 1 });
    expect(at["1.1"]).toMatchObject({ column: 2, row: 2 });
  });
});
