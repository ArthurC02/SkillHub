import type { BundleMemberChangeChangeEnum } from "@skillhub/api-client-ts";
import { isCategorizedFindings } from "../../core/api/findings";
import type { PublicPublication } from "./publishing.service";
import { isDate, isRecord } from "../../core/api/wire";
import { isAcquisition, isLabelled } from "./availability.model";

const MEMBER_CHANGES: ReadonlySet<string> = new Set<BundleMemberChangeChangeEnum>([
  "added",
  "removed",
  "changed",
]);

function isOptionalInteger(value: unknown) {
  return value === undefined || (typeof value === "number" && Number.isInteger(value));
}

function isMemberChange(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.name === "string" &&
    typeof value.change === "string" &&
    MEMBER_CHANGES.has(value.change) &&
    isOptionalInteger(value.from) &&
    isOptionalInteger(value.to)
  );
}

function isRelease(value: unknown) {
  return (
    isRecord(value) &&
    typeof value.content_hash === "string" &&
    isDate(value.released_at) &&
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
    isRecord(skill) &&
    typeof skill.name === "string" &&
    typeof skill.summary === "string" &&
    isRecord(release) &&
    isRelease(release) &&
    typeof release.version_number === "number" &&
    isCategorizedFindings(release.findings) &&
    isRecord(release.license) &&
    typeof release.license.expression === "string" &&
    typeof release.license.source === "string" &&
    isLabelled(release.redistribution)
  );
}

function isBundleContent(value: Record<string, unknown>) {
  const bundle = value.bundle;
  const release = value.bundle_release;
  return (
    isRecord(bundle) &&
    typeof bundle.version === "string" &&
    typeof bundle.description === "string" &&
    Array.isArray(bundle.members) &&
    bundle.members.every(
      (member) =>
        isRecord(member) &&
        typeof member.name === "string" &&
        typeof member.version_number === "number" &&
        Number.isInteger(member.version_number) &&
        typeof member.content_hash === "string",
    ) &&
    isRecord(release) &&
    isRelease(release) &&
    typeof release.version === "string" &&
    isCategorizedFindings(release.findings)
  );
}

export function readPublicPublication(value: unknown): PublicPublication {
  if (
    !isRecord(value) ||
    (value.kind !== "skill" && value.kind !== "bundle") ||
    typeof value.publisher !== "string" ||
    typeof value.name !== "string" ||
    typeof value.address !== "string" ||
    !isLabelled(value.availability) ||
    !Array.isArray(value.releases) ||
    !value.releases.every(isRelease) ||
    !isAcquisition(value.exposure) ||
    !isAcquisition(value.acquisition) ||
    (value.delisted_at !== undefined && !isDate(value.delisted_at))
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
