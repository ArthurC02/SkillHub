import type { ReactNode } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { SkillVersionPicker, SkillWorkspaceNav } from "../../../skill";
import type { useOwnSkills } from "../../../skill";
import type { useTestCase } from "../../testcases.service";

export function PreflightShell({
  skill,
  version,
  skillName,
  ownSkills,
  testCaseInfo,
  criteria,
  onPick,
  children,
}: {
  skill: string;
  version: string;
  skillName: string | undefined;
  ownSkills: ReturnType<typeof useOwnSkills>;
  testCaseInfo: ReturnType<typeof useTestCase>;
  criteria: number | undefined;
  onPick: (id: string) => void;
  children: ReactNode;
}) {
  return (
    <section>
      <h1>執行前權限確認</h1>
      <SkillWorkspaceNav skillId={skill} versionId={version || undefined} />
      <p>
        Skill：
        <strong>
          {skillName ??
            (ownSkills.isPending ? "讀取中…" : ownSkills.error ? "讀取失敗" : "不在你的清單裡")}
        </strong>
        {" ・ "}
        Test Case：
        <strong>
          {testCaseInfo.data?.name ??
            (testCaseInfo.isPending ? "讀取中…" : testCaseInfo.error ? "讀取失敗" : "讀不到名稱")}
        </strong>
      </p>
      {ownSkills.error && <ReadFailure error={ownSkills.error} what="你的 Skill 清單" />}
      {testCaseInfo.error && <ReadFailure error={testCaseInfo.error} what="Test Case" />}
      {criteria === 0 && (
        <p className="note">
          這個 Test Case 沒有驗收條件，所以這次 Run 不會產生逐條判定。試跑本身照常執行。
        </p>
      )}
      <SkillVersionPicker skillId={skill} value={version} onPick={onPick} />
      {children}
    </section>
  );
}
