import type { SkillRedistributionEnum } from "@skillhub/api-client-ts";
import { ListSkillsLimitMaximum } from "@skillhub/api-client-ts/limits";
import { contractEnum } from "../../core/api/contract-enum";
import type { OwnSkill, OwnSkills, OwnSkillTakedown, Skill } from "../../core/api/types";
import { isSkillRisk } from "./search-response.model";

const SKILL_REDISTRIBUTIONS = contractEnum<SkillRedistributionEnum>({
  allowed: true,
  blocked: true,
  unknown: true,
  self_supplied: true,
  generated: true,
});

function isSkillId(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)
  );
}

export function isSkillReceipt(value: unknown): value is Skill {
  if (!value || typeof value !== "object") return false;
  const receipt = value as Partial<Skill>;
  return (
    isSkillId(receipt.skill_id) &&
    typeof receipt.name === "string" &&
    typeof receipt.summary === "string" &&
    SKILL_REDISTRIBUTIONS.includes(receipt.redistribution ?? "")
  );
}

const isTimestamp = (value: unknown) =>
  typeof value === "string" && Number.isFinite(Date.parse(value));

function isOwnSkillTakedown(value: unknown): value is OwnSkillTakedown {
  if (!value || typeof value !== "object") return false;
  const takedown = value as Partial<OwnSkillTakedown>;
  return (
    isTimestamp(takedown.taken_down_at) &&
    (takedown.by === "owner"
      ? isTimestamp(takedown.restorable_until)
      : takedown.by === "operator" && takedown.restorable_until === undefined)
  );
}

function isOwnSkill(value: unknown): value is OwnSkill {
  if (!isSkillReceipt(value)) return false;
  const skill = value as Partial<OwnSkill>;
  const verification = skill.verification;
  return (
    (skill.takedown === undefined || isOwnSkillTakedown(skill.takedown)) &&
    (skill.access_restriction === undefined ||
      skill.access_restriction === null ||
      typeof skill.access_restriction === "string") &&
    (skill.forked_from_skill_id === undefined || isSkillId(skill.forked_from_skill_id)) &&
    (skill.forked_from_version_id === undefined || isSkillId(skill.forked_from_version_id)) &&
    isSkillRisk(skill.risk) &&
    verification !== null &&
    typeof verification === "object" &&
    ["scanned", "not_measured", "not_applicable"].includes(verification.value) &&
    typeof verification.label === "string" &&
    typeof verification.note === "string" &&
    (verification.scanned_at === undefined ||
      verification.scanned_at === null ||
      (typeof verification.scanned_at === "string" &&
        Number.isFinite(Date.parse(verification.scanned_at))))
  );
}

function isOwnSkillPageLimit(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= 1 &&
    value <= ListSkillsLimitMaximum
  );
}

export function readOwnSkillPage(value: unknown, offset = 0): OwnSkills {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("Invalid workspace skill list response");
  const page = value as Partial<OwnSkills>;
  if (
    !Array.isArray(page.skills) ||
    !page.skills.every(isOwnSkill) ||
    !isOwnSkillPageLimit(page.limit) ||
    typeof page.total !== "number" ||
    !Number.isSafeInteger(page.total) ||
    page.total < page.skills.length ||
    typeof page.truncated !== "boolean" ||
    page.skills.length > page.limit ||
    new Set(page.skills.map((skill) => skill.skill_id.toLowerCase())).size !== page.skills.length ||
    (page.truncated && (page.skills.length === 0 || page.total <= page.skills.length)) ||
    (offset === 0 && !page.truncated && page.total !== page.skills.length)
  )
    throw new Error("Invalid workspace skill list response");
  return page as OwnSkills;
}

export function ownSkillsFromPages(pages: OwnSkills[] | undefined): OwnSkills | undefined {
  const last = pages?.at(-1);
  if (!last) return undefined;
  const skills = new Map(
    pages?.flatMap((page) => page.skills).map((skill) => [skill.skill_id.toLowerCase(), skill]),
  );
  return { ...last, skills: Array.from(skills.values()) };
}
