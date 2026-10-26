import { describe, expect, test } from "vitest";
import { OWN_BUNDLE } from "../../testing/fixtures/platform";
import { readOwnBundleOverview, readOwnBundles } from "./owner-bundles.model";

const publication = {
  publisher: "acme-tools",
  name: "pdf-toolkit",
  address: "/p/acme-tools/pdf-toolkit",
  status: "published",
  status_changed_at: "2026-09-10T00:00:00Z",
  latest_release: {
    bundle_version: "1.1.0",
    content_hash: "sha256:bundle-2",
    released_at: "2026-09-10T00:00:00Z",
  },
};

const overview = { bundle: OWN_BUNDLE.bundle, latest_version: OWN_BUNDLE, publication };
const malformedVersions: [string, unknown][] = [
  ["null version", null],
  ["array version", []],
  ["missing bundle name", { ...OWN_BUNDLE, bundle: undefined }],
  ["non-text version", { ...OWN_BUNDLE, version: 1 }],
  ["missing description", { ...OWN_BUNDLE, description: undefined }],
  ["non-text digest", { ...OWN_BUNDLE, content_hash: null }],
  ["invalid creation time", { ...OWN_BUNDLE, created_at: "not a date" }],
  ["non-text creation time", { ...OWN_BUNDLE, created_at: 0 }],
  ["missing members", { ...OWN_BUNDLE, members: undefined }],
  ["non-list members", { ...OWN_BUNDLE, members: {} }],
  ["null member", { ...OWN_BUNDLE, members: [null] }],
  [
    "invalid skill identity",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], skill_id: "bad" }] },
  ],
  [
    "invalid version identity",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], version_id: "bad" }] },
  ],
  [
    "missing member name",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], name: undefined }] },
  ],
  [
    "fractional member version",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], version_number: 1.5 }] },
  ],
  [
    "non-numeric member version",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], version_number: "1" }] },
  ],
  [
    "missing member digest",
    { ...OWN_BUNDLE, members: [{ ...OWN_BUNDLE.members[0], content_hash: undefined }] },
  ],
];

for (const [name, read, row] of [
  ["Bundle versions", readOwnBundles, OWN_BUNDLE],
  ["Bundle overview", readOwnBundleOverview, overview],
] as const) {
  describe(name, () => {
    test("preserves a populated response", () => {
      const response = { bundles: [row] };
      expect(read(response)).toBe(response);
    });

    test("preserves a confirmed empty response", () => {
      const response = { bundles: [] };
      expect(read(response)).toBe(response);
    });

    test.each([
      ["null envelope", null],
      ["array envelope", []],
      ["missing collection", {}],
      ["non-list collection", { bundles: {} }],
      ["null row", { bundles: [null] }],
      ["one malformed row among valid rows", { bundles: [row, null] }],
    ])("rejects %s", (_name, value) => {
      expect(() => read(value)).toThrowError();
    });
  });
}

test.each(malformedVersions)("rejects %s in the version list", (_name, version) => {
  expect(() => readOwnBundles({ bundles: [version] })).toThrowError("無法確認 Bundle 清單的資料。");
});

test.each(malformedVersions)("rejects %s in the overview", (_name, latest_version) => {
  expect(() => readOwnBundleOverview({ bundles: [{ ...overview, latest_version }] })).toThrowError(
    "無法確認 Bundle 發佈概覽的資料。",
  );
});

test("accepts the contract's empty member array", () => {
  const response = { bundles: [{ ...OWN_BUNDLE, members: [] }] };
  expect(readOwnBundles(response)).toBe(response);
});

test.each(["published", "delisted"])("accepts a %s Bundle projection", (status) => {
  const response = { bundles: [{ ...overview, publication: { ...publication, status } }] };
  expect(readOwnBundleOverview(response)).toBe(response);
});

test("accepts an unpublished Bundle with no optional projection", () => {
  const response = { bundles: [{ bundle: OWN_BUNDLE.bundle, latest_version: OWN_BUNDLE }] };
  expect(readOwnBundleOverview(response)).toBe(response);
});

test("accepts an older projection with no optional Release or offer", () => {
  const response = {
    bundles: [{ ...overview, publication: { ...publication, latest_release: undefined } }],
  };
  expect(readOwnBundleOverview(response)).toBe(response);
});

test.each([true, false])("accepts acquisition available=%s", (available) => {
  const response = {
    bundles: [
      {
        ...overview,
        publication: {
          ...publication,
          acquisition: { available, note: "目前的取得條件" },
          availability: { value: "available", label: "提供中", note: "目前提供套件" },
        },
      },
    ],
  };
  expect(readOwnBundleOverview(response)).toBe(response);
});

const malformedOverviews: [string, unknown][] = [
  ["missing Bundle identity", { ...overview, bundle: undefined }],
  ["missing latest version", { ...overview, latest_version: undefined }],
  ["null publication", { ...overview, publication: null }],
  ["non-object publication", { ...overview, publication: [] }],
  ...["publisher", "name", "address", "status_changed_at"].map<[string, unknown]>((key) => [
    "missing publication " + key,
    { ...overview, publication: { ...publication, [key]: undefined } },
  ]),
  [
    "unknown publication status",
    { ...overview, publication: { ...publication, status: "future" } },
  ],
  [
    "invalid status time",
    { ...overview, publication: { ...publication, status_changed_at: "invalid" } },
  ],
  ["null Release", { ...overview, publication: { ...publication, latest_release: null } }],
  ...["bundle_version", "content_hash", "released_at"].map<[string, unknown]>((key) => [
    "missing Release " + key,
    {
      ...overview,
      publication: {
        ...publication,
        latest_release: { ...publication.latest_release, [key]: undefined },
      },
    },
  ]),
  [
    "invalid Release time",
    {
      ...overview,
      publication: {
        ...publication,
        latest_release: { ...publication.latest_release, released_at: "invalid" },
      },
    },
  ],
  ["null availability", { ...overview, publication: { ...publication, availability: null } }],
  ...["value", "label", "note"].map<[string, unknown]>((key) => [
    "missing availability " + key,
    {
      ...overview,
      publication: {
        ...publication,
        availability: {
          value: "available",
          label: "提供中",
          note: "目前提供套件",
          [key]: undefined,
        },
      },
    },
  ]),
  ["null acquisition", { ...overview, publication: { ...publication, acquisition: null } }],
  [
    "non-boolean acquisition",
    {
      ...overview,
      publication: { ...publication, acquisition: { available: "false", note: "不可取得" } },
    },
  ],
  [
    "missing acquisition note",
    { ...overview, publication: { ...publication, acquisition: { available: true } } },
  ],
];

test.each(malformedOverviews)("rejects %s in an owner overview", (_name, value) => {
  expect(() => readOwnBundleOverview({ bundles: [value] })).toThrowError(
    "無法確認 Bundle 發佈概覽的資料。",
  );
});
