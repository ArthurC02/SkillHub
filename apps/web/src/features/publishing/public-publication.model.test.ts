import { expect, test } from "vitest";
import { PUBLIC_BUNDLE_PUBLICATION, PUBLIC_PUBLICATION } from "../../testing/fixtures/platform";
import { readPublicPublication } from "./public-publication.model";

function changed(source: unknown, path: string, value: unknown) {
  const body = structuredClone(source) as Record<string, unknown>;
  const keys = path.split(".");
  let target = body;
  for (const key of keys.slice(0, -1)) target = target[key] as Record<string, unknown>;
  target[keys.at(-1)!] = value;
  return body;
}

const invalid: [string, unknown][] = [
  ["null", null],
  ["array", []],
  ["primitive", "available"],
  ["missing required fields", {}],
  ["unknown kind", { ...PUBLIC_PUBLICATION, kind: "other" }],
  ["missing publisher", changed(PUBLIC_PUBLICATION, "publisher", undefined)],
  ["invalid name", changed(PUBLIC_PUBLICATION, "name", 1)],
  ["invalid address", changed(PUBLIC_PUBLICATION, "address", null)],
  ["missing availability", changed(PUBLIC_PUBLICATION, "availability", undefined)],
  ["missing availability label", changed(PUBLIC_PUBLICATION, "availability.label", undefined)],
  ["invalid availability note", changed(PUBLIC_PUBLICATION, "availability.note", {})],
  ["invalid availability value", changed(PUBLIC_PUBLICATION, "availability.value", false)],
  ["missing release history", changed(PUBLIC_PUBLICATION, "releases", undefined)],
  ["invalid history entry", changed(PUBLIC_PUBLICATION, "releases", [null])],
  ["invalid history timestamp", changed(PUBLIC_PUBLICATION, "releases.0.released_at", "invalid")],
  ["invalid history hash", changed(PUBLIC_PUBLICATION, "releases.0.content_hash", {})],
  ["fractional version", changed(PUBLIC_PUBLICATION, "release.version_number", 1.5)],
  ["invalid changes", changed(PUBLIC_BUNDLE_PUBLICATION, "releases.0.changes", {})],
  [
    "unknown member change",
    changed(PUBLIC_BUNDLE_PUBLICATION, "releases.0.changes.0.change", "other"),
  ],
  [
    "invalid member change name",
    changed(PUBLIC_BUNDLE_PUBLICATION, "releases.0.changes.0.name", null),
  ],
  [
    "invalid earlier member version",
    changed(PUBLIC_BUNDLE_PUBLICATION, "releases.0.changes.0.from", 1.5),
  ],
  [
    "invalid later member version",
    changed(PUBLIC_BUNDLE_PUBLICATION, "releases.0.changes.0.to", "3"),
  ],
  ["missing exposure", changed(PUBLIC_PUBLICATION, "exposure", undefined)],
  ["invalid exposure flag", changed(PUBLIC_PUBLICATION, "exposure.available", "false")],
  ["missing acquisition note", changed(PUBLIC_PUBLICATION, "acquisition.note", undefined)],
  ["invalid withdrawal timestamp", changed(PUBLIC_PUBLICATION, "delisted_at", "invalid")],
  ["missing skill", changed(PUBLIC_PUBLICATION, "skill", undefined)],
  ["missing skill name", changed(PUBLIC_PUBLICATION, "skill.name", undefined)],
  ["invalid skill summary", changed(PUBLIC_PUBLICATION, "skill.summary", {})],
  ["missing skill release", changed(PUBLIC_PUBLICATION, "release", undefined)],
  ["missing current version", changed(PUBLIC_PUBLICATION, "release.version_number", undefined)],
  ["missing findings", changed(PUBLIC_PUBLICATION, "release.findings", undefined)],
  ["invalid finding", changed(PUBLIC_PUBLICATION, "release.findings.warnings", [null])],
  ["missing license", changed(PUBLIC_PUBLICATION, "release.license", undefined)],
  ["invalid license expression", changed(PUBLIC_PUBLICATION, "release.license.expression", {})],
  ["invalid license source", changed(PUBLIC_PUBLICATION, "release.license.source", [])],
  ["missing redistribution", changed(PUBLIC_PUBLICATION, "release.redistribution", undefined)],
  ["missing bundle", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle", undefined)],
  ["missing bundle version", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.version", undefined)],
  ["invalid bundle description", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.description", {})],
  ["missing bundle members", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.members", undefined)],
  ["invalid bundle member", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.members", [null])],
  [
    "fractional member version",
    changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.members.0.version_number", 1.5),
  ],
  ["invalid member hash", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle.members.0.content_hash", {})],
  ["missing bundle release", changed(PUBLIC_BUNDLE_PUBLICATION, "bundle_release", undefined)],
  [
    "missing bundle release version",
    changed(PUBLIC_BUNDLE_PUBLICATION, "bundle_release.version", undefined),
  ],
  [
    "missing bundle findings",
    changed(PUBLIC_BUNDLE_PUBLICATION, "bundle_release.findings", undefined),
  ],
];

test.each(invalid)("public read rejects %s instead of returning displayable data", (_, value) => {
  expect(() => readPublicPublication(value)).toThrow(Error);
});

test.each([
  ["skill", PUBLIC_PUBLICATION],
  ["bundle", PUBLIC_BUNDLE_PUBLICATION],
  [
    "empty optional text and collections",
    {
      ...PUBLIC_BUNDLE_PUBLICATION,
      bundle: { version: "", description: "", members: [] },
      releases: [],
      extra_metadata: { retained: true },
    },
  ],
  [
    "unavailable without content",
    {
      kind: "skill",
      publisher: "demo",
      name: "pdf-summariser",
      address: "/p/demo/pdf-summariser",
      availability: {
        value: "delisted",
        label: "作者已撤回",
        note: "作者撤回了這個發佈物，這一頁不再提供它的內容。",
      },
      delisted_at: "2026-10-20T18:00:00Z",
      releases: [],
      exposure: { available: false, note: "" },
      acquisition: { available: false, note: "" },
    },
  ],
] as const)("public read preserves valid %s data", (_, value) => {
  expect(readPublicPublication(value)).toBe(value);
});
