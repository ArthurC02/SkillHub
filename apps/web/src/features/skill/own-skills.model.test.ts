import { expect, test } from "vitest";
import type { OwnSkills } from "../../core/api/types";
import { platformResponse } from "../../testing/fixtures/platform";
import { ownSkillsFromPages } from "./own-skills.model";

test("no asset pages remains an unread list", () => {
  expect(ownSkillsFromPages(undefined)).toBeUndefined();
  expect(ownSkillsFromPages([])).toBeUndefined();
});

test("asset pages keep the final coverage and replace duplicate records in their original order", () => {
  const base = platformResponse("/skills").body as OwnSkills;
  const first = { ...base.skills[0], skill_id: "first", name: "First" };
  const second = { ...base.skills[0], skill_id: "second", name: "Second" };
  const result = ownSkillsFromPages([
    { skills: [first, second], total: 3, limit: 24, truncated: true },
    {
      skills: [
        { ...second, name: "Second updated" },
        { ...first, skill_id: "third", name: "Third" },
      ],
      total: 3,
      limit: 24,
      truncated: false,
    },
  ]);
  expect(result?.skills.map((skill) => [skill.skill_id, skill.name])).toEqual([
    ["first", "First"],
    ["second", "Second updated"],
    ["third", "Third"],
  ]);
  expect(result?.truncated).toBe(false);
  expect(result?.total).toBe(3);
  expect(result?.limit).toBe(24);
});

test("asset page identity deduplication ignores UUID letter case", () => {
  const base = platformResponse("/skills").body as OwnSkills;
  const id = "abcdef01-2345-4789-abcd-0123456789ab";
  const result = ownSkillsFromPages([
    {
      ...base,
      skills: [{ ...base.skills[0], skill_id: id, name: "First" }],
      total: 2,
      truncated: true,
    },
    {
      ...base,
      skills: [{ ...base.skills[0], skill_id: id.toUpperCase(), name: "Current" }],
      total: 1,
    },
  ]);
  expect(result?.skills.map((skill) => skill.name)).toEqual(["Current"]);
});
