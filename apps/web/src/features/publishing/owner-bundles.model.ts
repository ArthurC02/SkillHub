import type { BundleVersion, OwnerBundleSummary } from "./publishing.service";
import { isDate, isRecord, isUuid } from "../../core/api/wire";
import { isAcquisition, isLabelled } from "./availability.model";

function isMember(value: unknown) {
  return (
    isRecord(value) &&
    isUuid(value.skill_id) &&
    isUuid(value.version_id) &&
    typeof value.name === "string" &&
    typeof value.version_number === "number" &&
    Number.isInteger(value.version_number) &&
    typeof value.content_hash === "string"
  );
}

function isVersion(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.bundle === "string" &&
    typeof value.version === "string" &&
    typeof value.description === "string" &&
    typeof value.content_hash === "string" &&
    isDate(value.created_at) &&
    Array.isArray(value.members) &&
    value.members.every(isMember)
  );
}

function isOptionalRelease(value: unknown) {
  return (
    value === undefined ||
    (isRecord(value) &&
      typeof value.bundle_version === "string" &&
      typeof value.content_hash === "string" &&
      isDate(value.released_at))
  );
}

function isOptionalPublication(value: unknown) {
  if (value === undefined) return true;
  if (!isRecord(value)) return false;
  const availability = value.availability;
  const acquisition = value.acquisition;
  return (
    typeof value.publisher === "string" &&
    typeof value.name === "string" &&
    typeof value.address === "string" &&
    (value.status === "published" || value.status === "delisted") &&
    isDate(value.status_changed_at) &&
    isOptionalRelease(value.latest_release) &&
    (availability === undefined || isLabelled(availability)) &&
    (acquisition === undefined || isAcquisition(acquisition))
  );
}

function isOverview(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.bundle === "string" &&
    isVersion(value.latest_version) &&
    isOptionalPublication(value.publication)
  );
}

export function readOwnBundles(value: unknown): { bundles: BundleVersion[] } {
  if (!isRecord(value) || !Array.isArray(value.bundles) || !value.bundles.every(isVersion))
    throw new Error("無法確認 Bundle 清單的資料。");
  return value as unknown as { bundles: BundleVersion[] };
}

export function readOwnBundleOverview(value: unknown): { bundles: OwnerBundleSummary[] } {
  if (!isRecord(value) || !Array.isArray(value.bundles) || !value.bundles.every(isOverview))
    throw new Error("無法確認 Bundle 發佈概覽的資料。");
  return value as unknown as { bundles: OwnerBundleSummary[] };
}
