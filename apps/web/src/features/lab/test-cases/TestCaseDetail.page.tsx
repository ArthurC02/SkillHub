import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import { useTestCase } from "../testcases.service";
import { useRuns } from "../../runs";
import { RunHistory } from "./components/RunHistory";
import { DeleteTestCase } from "./components/DeleteTestCase";
import { PromptForm } from "./components/PromptForm";
import { CriteriaSection } from "./components/CriteriaSection";
import { RubricSection } from "./components/RubricSection";
import { DatasetSection } from "./components/DatasetSection";

export function TestCaseDetail() {
  const { testCaseId } = useParams({ from: "/lab/test-cases/$testCaseId" });
  const testCase = useTestCase(testCaseId);
  const runs = useRuns(testCaseId);
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
  const lastVersion = history[0]?.skill_version_id;

  return (
    <section key={testCaseId}>
      <h1>{testCase.data.name}</h1>
      <p className="note">
        <Link to="/lab/test-cases" search={{ skill: testCase.data.skill_id }}>
          回到這個 Skill 的 Test Case 列表
        </Link>
      </p>
      <PromptForm testCase={testCase.data} />
      <CriteriaSection testCase={testCase.data} />
      <RubricSection testCase={testCase.data} />
      <DatasetSection testCaseId={testCaseId} />
      <h2>開始試跑</h2>
      <p>
        <Link
          className="action"
          to="/lab/run"
          search={{ skill: testCase.data.skill_id, test_case: testCaseId, version: lastVersion }}
        >
          前往執行前權限確認
        </Link>
      </p>
      <p className="note" data-role="evidence">
        開始 Run 前會再次顯示權限摘要並要求確認。
      </p>
      <RunHistory runs={runs} history={history} />
      <DeleteTestCase testCaseId={testCaseId} onDeleted={setDeleted} />
    </section>
  );
}
