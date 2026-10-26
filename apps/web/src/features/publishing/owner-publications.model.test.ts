import { expect, test } from "vitest";
import { OWN_PUBLICATIONS } from "../../testing/fixtures/platform";
import { readOwnerPublications } from "./owner-publications.model";

function changed(path: string, value: unknown) {
  const body = structuredClone(OWN_PUBLICATIONS) as unknown as Record<string, unknown>;
  const keys = path.split(".");
  let target = body;
  for (const key of keys.slice(0, -1)) target = target[key] as Record<string, unknown>;
  target[keys.at(-1)!] = value;
  return body;
}

test.each([
  ["populated", OWN_PUBLICATIONS],
  ["confirmed empty", { publications: [] }],
  ["older server", changed("publications.0.availability", undefined)],
  ["missing acquisition projection", changed("publications.0.acquisition", undefined)],
  ["unreleased", changed("publications.0.latest_release", undefined)],
  ["withdrawn", changed("publications.0.status", "delisted")],
  ["unavailable", changed("publications.0.acquisition.available", false)],
] as const)("preserves the complete %s publication response", (_, value) => {
  expect(readOwnerPublications(value)).toEqual(value);
});

test.each([
  ["null envelope", null],
  ["array envelope", []],
  ["missing list", {}],
  ["null list", { publications: null }],
  ["non-array list", { publications: {} }],
  ["null row", { publications: [null] }],
  ["non-record row", { publications: ["published"] }],
  ["invalid identity", changed("publications.0.skill_id", "not-a-uuid")],
  ["missing publisher", changed("publications.0.publisher", undefined)],
  ["invalid name", changed("publications.0.name", {})],
  ["invalid address", changed("publications.0.address", null)],
  ["unknown publication status", changed("publications.0.status", "future_status")],
  ["invalid status timestamp", changed("publications.0.status_changed_at", "invalid")],
  ["missing exposure", changed("publications.0.catalog_exposure", undefined)],
  ["unknown exposure", changed("publications.0.catalog_exposure.state", "future_state")],
  ["null release", changed("publications.0.latest_release", null)],
  ["invalid release identity", changed("publications.0.latest_release.version_id", "other")],
  ["fractional release version", changed("publications.0.latest_release.version_number", 1.5)],
  ["string release version", changed("publications.0.latest_release.version_number", "2")],
  ["invalid release timestamp", changed("publications.0.latest_release.released_at", "invalid")],
  ["null availability", changed("publications.0.availability", null)],
  ["missing availability value", changed("publications.0.availability.value", undefined)],
  ["invalid availability label", changed("publications.0.availability.label", 1)],
  ["invalid availability note", changed("publications.0.availability.note", {})],
  ["null acquisition", changed("publications.0.acquisition", null)],
  ["truthy acquisition string", changed("publications.0.acquisition.available", "false")],
  ["missing acquisition note", changed("publications.0.acquisition.note", undefined)],
  ["mixed valid and invalid rows", { publications: [...OWN_PUBLICATIONS.publications, null] }],
] as const)("rejects %s instead of treating it as a publication observation", (_, value) => {
  expect(() => readOwnerPublications(value)).toThrow("無法確認小工具發佈清單的資料。");
});
