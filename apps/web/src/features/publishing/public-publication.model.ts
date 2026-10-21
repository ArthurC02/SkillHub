import { isCategorizedFindings } from "../../core/api/findings";
import type { PublicPublication } from "./publishing.service";

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isLabelled(value: unknown) {
  return (
    isObject(value) &&
    typeof value.value === "string" &&
    typeof value.label === "string" &&
    typeof value.note === "string"
  );
}

function isPublicationNote(value: unknown) {
  return isObject(value) && typeof value.available === "boolean" && typeof value.note === "string";
}

function isOptionalInteger(value: unknown) {
  return value === undefined || (typeof value === "number" && Number.isInteger(value));
}

function isMemberChange(value: unknown) {
  return (
    isObject(value) &&
    typeof value.name === "string" &&
    typeof value.change === "string" &&
    ["added", "removed", "changed"].includes(value.change) &&
    isOptionalInteger(value.from) &&
    isOptionalInteger(value.to)
  );
}

function isRelease(value: unknown) {
  return (
    isObject(value) &&
    typeof value.content_hash === "string" &&
    typeof value.released_at === "string" &&
    Number.isFinite(Date.parse(value.released_at)) &&
    isOptionalInteger(value.version_number) &&
    (value.version === undefined || typeof value.version === "string") &&
    (value.changes === undefined ||
      (Array.isArray(value.changes) && value.changes.every(isMemberChange)))
  );
}

function isSkillContent(value: Record<string, unknown>) {
  const skill = value.skill;
  const release = value.release;
  return (
    isObject(skill) &&
    typeof skill.name === "string" &&
    typeof skill.summary === "string" &&
    isObject(release) &&
    isRelease(release) &&
    typeof release.version_number === "number" &&
    isCategorizedFindings(release.findings) &&
    isObject(release.license) &&
    typeof release.license.expression === "string" &&
    typeof release.license.source === "string" &&
    isLabelled(release.redistribution)
  );
}

function isBundleContent(value: Record<string, unknown>) {
  const bundle = value.bundle;
  const release = value.bundle_release;
  return (
    isObject(bundle) &&
    typeof bundle.version === "string" &&
    typeof bundle.description === "string" &&
    Array.isArray(bundle.members) &&
    bundle.members.every(
      (member) =>
        isObject(member) &&
        typeof member.name === "string" &&
        typeof member.version_number === "number" &&
        Number.isInteger(member.version_number) &&
        typeof member.content_hash === "string",
    ) &&
    isObject(release) &&
    isRelease(release) &&
    typeof release.version === "string" &&
    isCategorizedFindings(release.findings)
  );
}

export function readPublicPublication(value: unknown): PublicPublication {
  if (
    !isObject(value) ||
    (value.kind !== "skill" && value.kind !== "bundle") ||
    typeof value.publisher !== "string" ||
    typeof value.name !== "string" ||
    typeof value.address !== "string" ||
    !isLabelled(value.availability) ||
    !Array.isArray(value.releases) ||
    !value.releases.every(isRelease) ||
    !isPublicationNote(value.exposure) ||
    !isPublicationNote(value.acquisition) ||
    (value.delisted_at !== undefined &&
      (typeof value.delisted_at !== "string" || !Number.isFinite(Date.parse(value.delisted_at))))
  )
    throw new Error("無法確認公開發佈物的資料。");

  const availability = value.availability as Record<string, unknown>;
  if (
    availability.value === "available" &&
    !(value.kind === "skill" ? isSkillContent(value) : isBundleContent(value))
  )
    throw new Error("無法確認公開發佈物的版本內容。");

  return value as unknown as PublicPublication;
}
