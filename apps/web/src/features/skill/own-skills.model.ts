import type { OwnSkills } from "../../core/api/types";

export function ownSkillsFromPages(pages: OwnSkills[] | undefined): OwnSkills | undefined {
  const last = pages?.at(-1);
  if (!last) return undefined;
  const skills = new Map(
    pages?.flatMap((page) => page.skills).map((skill) => [skill.skill_id, skill]),
  );
  return { ...last, skills: Array.from(skills.values()) };
}
