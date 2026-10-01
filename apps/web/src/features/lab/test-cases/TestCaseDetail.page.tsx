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
        <h1>已刪除這個測試題</h1>
        <p role="status">
          草稿與它的 {deleted.datasets_deleted} 個上傳檔案都已刪除，檔案本體也已移除。
        </p>
        <p className="note">
          <strong>快照與歷史試跑紀錄不受影響</strong>
          ：已經跑過的試跑紀錄仍保留當時凍結的
          Prompt、驗收條件，以及每個檔案的名稱與內容雜湊，所以那些
          試跑紀錄仍可追溯，只是不再可重現。
        </p>
        <p>
          <Link to="/lab/test-cases">回到測試題列表</Link>
        </p>
      </section>
    );
  }

  if (testCase.isPending) return <Loading what="測試題" />;
  if (testCase.error) {
    if (testCase.error instanceof ApiError && testCase.error.status === 404) {
      return <p role="alert">找不到這個測試題。</p>;
    }
    return <ReadFailure error={testCase.error} what="測試題" />;
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
    <section key={testCaseId} className="test-case-detail-page">
      <header className="test-case-header">
        <p className="page-eyebrow">Evaluation workbench</p>
        <h1>{testCase.data.name}</h1>
        <p>在同一個小工具與 Version 脈絡裡維護驗證設計、試跑與歷史證據。</p>
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
      <div className="test-case-detail-layout">
        <div className="test-case-design">
          <section className="test-case-work-panel" aria-label="任務提示設計">
            <PromptForm testCase={testCase.data} />
          </section>
          <section className="test-case-work-panel" aria-label="驗收與評分設計">
            <CriteriaSection testCase={testCase.data} />
            <RubricSection testCase={testCase.data} />
          </section>
        </div>
        <aside className="test-case-detail-rail" aria-label="測試資料與下一步">
          <section className="test-case-work-panel" aria-label="測試資料">
            <DatasetSection testCaseId={testCaseId} versionId={selectedVersion?.version_id} />
          </section>
          <section className="test-case-next-step" aria-labelledby="test-case-next-step-title">
            <p className="page-eyebrow">Ready to validate</p>
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
              開始試跑前會再次顯示權限摘要並要求確認。
            </p>
          </section>
        </aside>
      </div>
      <section className="test-case-history-panel" aria-label="執行歷史">
        <RunHistory runs={runs} history={history} />
      </section>
      <section className="test-case-danger-panel" aria-label="刪除測試題">
        <DeleteTestCase testCaseId={testCaseId} onDeleted={setDeleted} />
      </section>
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
    if (!selectedVersion) return "這個 Version 不屬於目前的小工具";
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
          <dt>小工具</dt>
          <dd>
            <Link to="/skills/$skillId" params={{ skillId }}>
              {skill.data?.name ??
                (skill.isPending ? "讀取中…" : skill.error ? "名稱讀取失敗" : "這個小工具")}
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
                  {versionSource === "url" ? "由這個網址選定" : "沿用最近一次試跑紀錄"}
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
          <dt>測試題</dt>
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
      <ReadFailure error={skill.error} what="這個小工具" />
      <ReadFailure error={versions.error} what="Version 清單" />
      <p className="test-case-context-return">
        <Link
          to="/lab/test-cases"
          search={{ skill: skillId, version: selectedVersion?.version_id }}
        >
          回到這個小工具的測試題列表
        </Link>
      </p>
    </section>
  );
}
