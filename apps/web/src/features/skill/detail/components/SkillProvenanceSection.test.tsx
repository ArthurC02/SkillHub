import { expect, test, vi } from "vitest";
import { skillDetail } from "../../../../testing/fixtures/platform";
import { SkillProvenanceSection } from "./SkillProvenanceSection";
import { mountedComponent } from "../../../../testing/component";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    to,
    params,
    children,
  }: {
    to: string;
    params?: Record<string, string>;
    children?: unknown;
  }) => (
    <a href={Object.entries(params ?? {}).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to)}>
      {children as never}
    </a>
  ),
}));

const { container, mount, text } = mountedComponent();

test("says there is no source record when the skill has none", async () => {
  const skill = skillDetail("s-1", "無來源小工具");
  skill.source = undefined;
  await mount(<SkillProvenanceSection skill={skill} />);

  expect(text()).toContain("沒有保存任何來源紀錄。");
});

test("links to the original skill when this one is a fork", async () => {
  const skill = skillDetail("s-2", "分岔的小工具");
  skill.source = undefined;
  skill.derivation = {
    is_fork: true,
    label: "分岔自另一個小工具",
    note: "這是複製品。",
    forked_from_skill_id: "s-1",
  };
  await mount(<SkillProvenanceSection skill={skill} />);

  expect(text()).toContain("查看原始小工具");
  const link = container().querySelector("a")!;
  expect(link.getAttribute("href")).toBe("/skills/s-1");
});

test("does not link to an original skill when this one is not a fork", async () => {
  const skill = skillDetail("s-3", "原生小工具");
  skill.source = undefined;
  skill.derivation = { is_fork: false, label: "來源關係", note: "不是複製品。" };
  await mount(<SkillProvenanceSection skill={skill} />);

  expect(container().querySelector("a")).toBeNull();
});

test("does not link when forked_from_skill_id is set but is_fork is false", async () => {
  const skill = skillDetail("s-4", "資料不一致的小工具");
  skill.source = undefined;
  skill.derivation = {
    is_fork: false,
    label: "來源關係",
    note: "不是複製品。",
    forked_from_skill_id: "s-1",
  };
  await mount(<SkillProvenanceSection skill={skill} />);

  expect(container().querySelector("a")).toBeNull();
});
