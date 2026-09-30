import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import type { SkillVersionSummary } from "../../../core/api/types";
import { useTestCase } from "../testcases.service";
import { useRuns } from "../../runs";
import { RunHistory } from "./components/RunHistory";
import { DeleteTestCase } from "./components/DeleteTestCase";
import { PromptForm } from "./components/PromptForm";
import { CriteriaSection } from "./components/CriteriaSection";
import { RubricSection } from "./components/RubricSection";
import { DatasetSection } from "./components/DatasetSection";
import { SkillWorkspaceNav, useSkillDetail, useSkillVersions } from "../../skill";
import "./TestCaseDetail.page.css";

export function TestCaseDetail() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId" });
  const { version } = useSearch({ from: "/lab/test-cases/$testCaseId" });
  const testCase = useTestCase(testCaseId);
  const runs = useRuns({ testCaseId });
  const skillId = testCase.data?.skill_id ?? "";
  const skill = useSkillDetail(skillId);
  const versions = useSkillVersions(skillId);
  const [deleted, setDeleted] = useState<{ datasets_deleted: number } | null>(null);

  if (deleted) {
    return (
      <section>
        <h1>已刪除這個 Test Case</h1>
        <p role="status">
          草稿與它的 {deleted.datasets_deleted} 個上傳檔案都已刪除，檔案本體也已移除。
        </p>
        <p className="note">
          <strong>快照與歷史 Run 不受影響</strong>
          ：已經跑過的 Run 仍保留當時凍結的 Prompt、驗收條件，以及每個檔案的名稱與內容雜湊，所以那些
          Run 仍可追溯，只是不再可重現。
        </p>
        <p>
          <Link to="/lab/test-cases">回到 Test Case 列表</Link>
        </p>
      </section>
    );
  }

  if (testCase.isPending) return <Loading what=" Test Case " />;
  if (testCase.error) {
    if (testCase.error instanceof ApiError && testCase.error.status === 404) {
      return <p role="alert">找不到這個 Test Case。</p>;
    }
    return <ReadFailure error={testCase.error} what=" Test Case" />;
  }

  const history = runs.data?.pages.flatMap((page) => page.runs) ?? [];
  const lastMatchingRun = history.find(
    (run) => run.skill_id === skillId && run.test_case_id === testCaseId,
  );
  const requestedVersion = version ?? lastMatchingRun?.skill_version_id;
  const selectedVersion = versions.data?.versions?.find(
    (candidate) => candidate.version_id === requestedVersion,
  );
  const versionSource = version ? "url" : lastMatchingRun ? "run" : "none";

  return (
    <section key={testCaseId}>
      <header className="test-case-header">
        <h1>{testCase.data.name}</h1>
        <p>在同一個 Skill 與 Version 脈絡裡維護驗證設計、試跑與歷史證據。</p>
      </header>
      <SkillWorkspaceNav
        skillId={skillId}
        versionId={selectedVersion?.version_id}
        testCaseId={testCaseId}
      />
      <TestCaseContext
        testCaseName={testCase.data.name}
        testCaseId={testCaseId}
        skillId={skillId}
        skill={skill}
        versions={versions}
        requestedVersion={requestedVersion}
        selectedVersion={selectedVersion}
        versionSource={versionSource}
        runsPending={runs.isPending}
        runsError={runs.error}
      />
      <PromptForm testCase={testCase.data} />
      <CriteriaSection testCase={testCase.data} />
      <RubricSection testCase={testCase.data} />
      <DatasetSection testCaseId={testCaseId} versionId={selectedVersion?.version_id} />
      <section className="test-case-next-step" aria-labelledby="test-case-next-step-title">
        <h2 id="test-case-next-step-title">下一步：試跑這個驗證設計</h2>
        <p>
          <Link
            className="action"
            to="/skills/$skillId/test-cases/$testCaseId/runs/new"
            params={{ skillId, testCaseId }}
            search={{ version: selectedVersion?.version_id }}
          >
            {selectedVersion ? "用這個版本試跑" : "選擇 Version 並確認權限"}
          </Link>
        </p>
        <p className="note" data-role="evidence">
          開始 Run 前會再次顯示權限摘要並要求確認。
        </p>
      </section>
      <RunHistory runs={runs} history={history} />
      <DeleteTestCase testCaseId={testCaseId} onDeleted={setDeleted} />
    </section>
  );
}

function TestCaseContext({
  testCaseName,
  testCaseId,
  skillId,
  skill,
  versions,
  requestedVersion,
  selectedVersion,
  versionSource,
  runsPending,
  runsError,
}: {
  testCaseName: string;
  testCaseId: string;
  skillId: string;
  skill: ReturnType<typeof useSkillDetail>;
  versions: ReturnType<typeof useSkillVersions>;
  requestedVersion: string | undefined;
  selectedVersion: SkillVersionSummary | undefined;
  versionSource: "url" | "run" | "none";
  runsPending: boolean;
  runsError: Error | null;
}) {
  const versionState = (() => {
    if (!requestedVersion) {
      if (runsPending) return "正在確認最近使用的 Version…";
      if (runsError) return "最近使用的 Version 無法確認";
      return "尚未選擇 Version";
    }
    if (versions.isPending) return "正在確認這個 Version…";
    if (versions.error) return "Version 資訊讀取失敗";
    if (!selectedVersion) return "這個 Version 不屬於目前的 Skill";
    return null;
  })();

  return (
    <section
      className="download-item test-case-context"
      aria-labelledby="test-case-context-title"
      data-role="test-case-context"
    >
      <h2 id="test-case-context-title">目前驗證脈絡</h2>
      <dl className="test-case-context-facts">
        <div>
          <dt>Skill</dt>
          <dd>
            <Link to="/skills/$skillId" params={{ skillId }}>
              {skill.data?.name ??
                (skill.isPending ? "讀取中…" : skill.error ? "名稱讀取失敗" : "這個 Skill")}
            </Link>
          </dd>
        </div>
        <div>
          <dt>Version</dt>
          <dd>
            {selectedVersion ? (
              <>
                <Link
                  to="/skills/$skillId/versions/$versionId"
                  params={{ skillId, versionId: selectedVersion.version_id }}
                >
                  v{selectedVersion.version_number}
                </Link>{" "}
                <span className="badge">
                  {versionSource === "url" ? "由這個網址選定" : "沿用最近一次 Run"}
                </span>
              </>
            ) : (
              <span
                role={
                  requestedVersion && !versions.isPending && !versions.error ? "alert" : "status"
                }
              >
                {versionState}
              </span>
            )}
          </dd>
        </div>
        <div>
          <dt>Test Case</dt>
          <dd>
            <Link
              to="/lab/test-cases/$testCaseId"
              params={{ testCaseId }}
              search={{ version: selectedVersion?.version_id }}
              aria-current="page"
            >
              {testCaseName}
            </Link>
          </dd>
        </div>
      </dl>
      <ReadFailure error={skill.error} what="這個 Skill" />
      <ReadFailure error={versions.error} what="Version 清單" />
      <p className="test-case-context-return">
        <Link
          to="/lab/test-cases"
          search={{ skill: skillId, version: selectedVersion?.version_id }}
        >
          回到這個 Skill 的 Test Case 列表
        </Link>
      </p>
    </section>
  );
}
