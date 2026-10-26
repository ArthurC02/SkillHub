import { isCategorizedFindings } from "../../core/api/findings";
import type { DeclarableLicense } from "@skillhub/api-client-ts";
import type { Publication, Publisher } from "./publishing.service";

export const DECLARABLE_LICENSES: readonly DeclarableLicense[] = [
  "Apache-2.0",
  "MIT",
  "ISC",
  "BSD-2-Clause",
  "BSD-3-Clause",
  "GPL-2.0",
  "GPL-3.0",
  "LGPL-3.0",
  "AGPL-3.0",
  "MPL-2.0",
  "Unlicense",
  "CC0-1.0",
  "CC-BY-4.0",
  "CC-BY-SA-4.0",
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isDate(value: unknown) {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

function isUuid(value: unknown) {
  return (
    typeof value === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)
  );
}

function isRelease(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.content_hash === "string" &&
    isDate(value.released_at) &&
    typeof value.rights_attested === "boolean" &&
    isCategorizedFindings(value.findings) &&
    (value.version_id === undefined || isUuid(value.version_id)) &&
    (value.version_number === undefined ||
      (typeof value.version_number === "number" && Number.isInteger(value.version_number))) &&
    (value.bundle_version === undefined || typeof value.bundle_version === "string") &&
    (value.declared_license === undefined ||
      DECLARABLE_LICENSES.some((license) => license === value.declared_license))
  );
}

function isLabelled(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.value === "string" &&
    typeof value.label === "string" &&
    typeof value.note === "string"
  );
}

function isAcquisition(value: unknown) {
  return isRecord(value) && typeof value.available === "boolean" && typeof value.note === "string";
}

export function readOwnPublisher(value: unknown): Publisher {
  if (!isRecord(value) || typeof value.name !== "string" || !isDate(value.created_at))
    throw new Error("無法確認發佈者資訊的資料。");
  return value as unknown as Publisher;
}

export function readOwnPublication(value: unknown): Publication {
  if (
    !isRecord(value) ||
    (value.kind !== "skill" && value.kind !== "bundle") ||
    typeof value.publisher !== "string" ||
    typeof value.name !== "string" ||
    typeof value.address !== "string" ||
    (value.status !== "published" && value.status !== "delisted") ||
    !isDate(value.status_changed_at) ||
    !Array.isArray(value.releases) ||
    !value.releases.every(isRelease) ||
    (value.availability !== undefined && !isLabelled(value.availability)) ||
    (value.acquisition !== undefined && !isAcquisition(value.acquisition))
  )
    throw new Error("無法確認小工具發佈狀態的資料。");
  return value as unknown as Publication;
}
