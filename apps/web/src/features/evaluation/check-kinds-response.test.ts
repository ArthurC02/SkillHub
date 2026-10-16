import { afterEach, expect, test, vi } from "vitest";
import {
  CriterionCheckChildrenMaxItems,
  CriterionCheckValuesMaxItems,
} from "@skillhub/api-client-ts/limits";
import { CHECK_KINDS } from "../../testing/fixtures/platform";
import { getCheckKinds } from "./evaluation.service";

afterEach(() => vi.unstubAllGlobals());

const kind = CHECK_KINDS.kinds[0];

test.each([
  ["current kinds", CHECK_KINDS],
  ["empty collection", { ...CHECK_KINDS, kinds: [] }],
  ["zero limits", { ...CHECK_KINDS, max_nodes: 0, max_depth: 0 }],
  [
    "largest safe limits",
    { ...CHECK_KINDS, max_nodes: Number.MAX_SAFE_INTEGER, max_depth: Number.MAX_SAFE_INTEGER },
  ],
  ["future kind", { ...CHECK_KINDS, kinds: [{ ...kind, kind: "future_kind" }] }],
  ["empty display strings", { ...CHECK_KINDS, kinds: [{ ...kind, label: "", description: "" }] }],
  [
    "zero bounds",
    {
      ...CHECK_KINDS,
      kinds: [{ ...kind, values: { min: 0, max: 0 }, children: { min: 0, max: 0 } }],
    },
  ],
  [
    "maximum value bounds",
    {
      ...CHECK_KINDS,
      kinds: [
        {
          ...kind,
          values: { min: CriterionCheckValuesMaxItems, max: CriterionCheckValuesMaxItems },
        },
      ],
    },
  ],
  [
    "maximum child bounds",
    {
      ...CHECK_KINDS,
      kinds: [
        {
          ...kind,
          category: "composite",
          children: { min: CriterionCheckChildrenMaxItems, max: CriterionCheckChildrenMaxItems },
        },
      ],
    },
  ],
  [
    "additional fields",
    { ...CHECK_KINDS, future_field: true, kinds: [{ ...kind, future_field: true }] },
  ],
])("a readable palette with %s is preserved", async (_label, body) => {
  const fetch = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
    Promise.resolve(new Response(JSON.stringify(body))),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(getCheckKinds()).resolves.toEqual(body);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]?.[0]).toBe("/evaluation/check-kinds");
  expect(fetch.mock.calls[0]?.[1]?.method ?? "GET").toBe("GET");
});

test.each<[string, unknown]>([
  ["null body", null],
  ["array body", []],
  ["primitive body", true],
  ["missing kinds", {}],
  ["null kinds", { ...CHECK_KINDS, kinds: null }],
  ["object kinds", { ...CHECK_KINDS, kinds: {} }],
  ["null kind", { ...CHECK_KINDS, kinds: [null] }],
  ["array kind", { ...CHECK_KINDS, kinds: [[]] }],
  ["primitive kind", { ...CHECK_KINDS, kinds: [true] }],
  ["missing kind name", { ...CHECK_KINDS, kinds: [{ ...kind, kind: undefined }] }],
  ["empty kind name", { ...CHECK_KINDS, kinds: [{ ...kind, kind: "" }] }],
  ["object kind name", { ...CHECK_KINDS, kinds: [{ ...kind, kind: {} }] }],
  ["missing category", { ...CHECK_KINDS, kinds: [{ ...kind, category: undefined }] }],
  ["unknown category", { ...CHECK_KINDS, kinds: [{ ...kind, category: "unknown" }] }],
  ["missing label", { ...CHECK_KINDS, kinds: [{ ...kind, label: undefined }] }],
  ["object label", { ...CHECK_KINDS, kinds: [{ ...kind, label: {} }] }],
  ["missing description", { ...CHECK_KINDS, kinds: [{ ...kind, description: undefined }] }],
  ["object description", { ...CHECK_KINDS, kinds: [{ ...kind, description: {} }] }],
  ["missing at least flag", { ...CHECK_KINDS, kinds: [{ ...kind, at_least: undefined }] }],
  ["string at least flag", { ...CHECK_KINDS, kinds: [{ ...kind, at_least: "false" }] }],
  ["missing node limit", { ...CHECK_KINDS, max_nodes: undefined }],
  ["missing depth limit", { ...CHECK_KINDS, max_depth: undefined }],
  ["null node limit", { ...CHECK_KINDS, max_nodes: null }],
  ["string depth limit", { ...CHECK_KINDS, max_depth: "8" }],
  ["fractional node limit", { ...CHECK_KINDS, max_nodes: 1.5 }],
  ["unsafe depth limit", { ...CHECK_KINDS, max_depth: Number.MAX_SAFE_INTEGER + 1 }],
  ...(["values", "children"] as const).flatMap((field): [string, unknown][] => [
    [`missing ${field} bounds`, { ...CHECK_KINDS, kinds: [{ ...kind, [field]: undefined }] }],
    [`null ${field} bounds`, { ...CHECK_KINDS, kinds: [{ ...kind, [field]: null }] }],
    [`array ${field} bounds`, { ...CHECK_KINDS, kinds: [{ ...kind, [field]: [] }] }],
    [`missing ${field} minimum`, { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { max: 1 } }] }],
    [`missing ${field} maximum`, { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: 0 } }] }],
    [
      `string ${field} minimum`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: "0", max: 1 } }] },
    ],
    [
      `string ${field} maximum`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: 0, max: "1" } }] },
    ],
    [
      `negative ${field} minimum`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: -1, max: 1 } }] },
    ],
    [
      `inverted ${field} bounds`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: 1, max: 0 } }] },
    ],
    [
      `fractional ${field} minimum`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: 0.5, max: 1 } }] },
    ],
    [
      `fractional ${field} maximum`,
      { ...CHECK_KINDS, kinds: [{ ...kind, [field]: { min: 0, max: 1.5 } }] },
    ],
    [
      `excessive ${field} maximum`,
      {
        ...CHECK_KINDS,
        kinds: [
          {
            ...kind,
            [field]: {
              min: 0,
              max:
                (field === "values"
                  ? CriterionCheckValuesMaxItems
                  : CriterionCheckChildrenMaxItems) + 1,
            },
          },
        ],
      },
    ],
  ]),
])("a palette with %s is a read failure", async (_label, body) => {
  vi.stubGlobal("fetch", () => Promise.resolve(new Response(JSON.stringify(body))));
  await expect(getCheckKinds()).rejects.toThrow("Invalid check kinds response");
});
