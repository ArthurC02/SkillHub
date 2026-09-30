import type { ReactNode } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { SkillVersionPicker, SkillWorkspaceNav } from "../../../skill";
import type { useSkillDetail } from "../../../skill";
import type { useTestCase } from "../../testcases.service";

export function PreflightShell({
  skill,
  version,
  skillInfo,
  testCaseInfo,
  criteria,
  onPick,
  children,
}: {
  skill: string;
  version: string;
  skillInfo: ReturnType<typeof useSkillDetail>;
  testCaseInfo: ReturnType<typeof useTestCase>;
  criteria: number | undefined;
  onPick: (id: string) => void;
  children: ReactNode;
}) {
  return (
    <section className="preflight-page">
      <header className="preflight-header">
        <p className="page-eyebrow">Run safety gate</p>
        <h1>執行前權限確認</h1>
        <p>在建立 Run 之前，先核對版本、可接觸的資料、工具與資源上限。</p>
      </header>
      <SkillWorkspaceNav
        skillId={skill}
        versionId={version || undefined}
        testCaseId={testCaseInfo.data?.test_case_id}
      />
      <section className="preflight-context" aria-labelledby="run-context-title">
        <h2 id="run-context-title">這次 Run 的脈絡</h2>
        <p>
          Skill：
          <strong>
            {skillInfo.data?.name ??
              (skillInfo.isPending ? "讀取中…" : skillInfo.error ? "讀取失敗" : "讀不到名稱")}
          </strong>
          {" ・ "}
          Test Case：
          <strong>
            {testCaseInfo.data?.name ??
              (testCaseInfo.isPending ? "讀取中…" : testCaseInfo.error ? "讀取失敗" : "讀不到名稱")}
          </strong>
        </p>
        <SkillVersionPicker skillId={skill} value={version} onPick={onPick} />
      </section>
      {skillInfo.error && <ReadFailure error={skillInfo.error} what="這個 Skill" />}
      {testCaseInfo.error && <ReadFailure error={testCaseInfo.error} what="Test Case" />}
      {criteria === 0 && (
        <p className="note">
          這個 Test Case 沒有驗收條件，所以這次 Run 不會產生逐條判定。試跑本身照常執行。
        </p>
      )}
      <section className="preflight-body" aria-label="這次 Run 的權限與資源摘要">
        {children}
      </section>
    </section>
  );
}
