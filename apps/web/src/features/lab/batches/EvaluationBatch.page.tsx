import { Link, useParams, useSearch } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ApiError } from "../../../core/api/client";
import { SkillWorkspaceNav, useSkillVersions } from "../../skill";
import { useTestCases } from "../testcases.service";
import { useEvaluationBatchReport } from "./batches.service";
import { ArmSummary } from "./components/ArmSummary";
import { TestCaseResults } from "./components/TestCaseResults";
import { PairedSummary } from "./components/PairedSummary";
import { BatchState } from "./components/BatchState";
import { BatchComparison } from "./components/BatchComparison";

const HEADING = "evaluation-batch-heading";

export function EvaluationBatch() {
  const { skillId, batchId } = useParams({ from: "/skills/$skillId/evaluation-batches/$batchId" });
  const { against = "" } = useSearch({ from: "/skills/$skillId/evaluation-batches/$batchId" });
  const report = useEvaluationBatchReport(skillId, batchId);
  const versions = useSkillVersions(skillId);
  const testCases = useTestCases(skillId);
  const data = report.error ? undefined : report.data;
  const names = new Map(
    (testCases.error ? [] : (testCases.data?.pages.flatMap((p) => p.test_cases) ?? [])).map(
      (row) => [row.test_case_id.toLowerCase(), row.name],
    ),
  );
  const versionNumber = data
    ? versions.data?.versions.find(
        (v) => v.version_id.toLowerCase() === data.batch.skill_version_id.toLowerCase(),
      )?.version_number
    : undefined;
  const gone = report.error instanceof ApiError && report.error.status === 404;

  return (
    <section>
      <h1 id={HEADING} tabIndex={-1}>
        評估批次結果
      </h1>
      <SkillWorkspaceNav skillId={skillId} versionId={data?.batch.skill_version_id} />
      <p>
        <Link
          to="/skills/$skillId/evaluation-batches"
          params={{ skillId }}
          search={{ version: data?.batch.skill_version_id }}
        >
          回到這個小工具的評估批次
        </Link>
      </p>
      {report.isPending && <Loading what="評估批次" paused={report.fetchStatus === "paused"} />}
      <ReadFailure
        error={report.error}
        what="評估批次"
        retry={gone ? undefined : () => void report.refetch()}
        retrying={report.fetchStatus !== "idle"}
        focusTarget={HEADING}
      >
        {gone && <p role="alert">找不到這個評估批次，或它不屬於這個小工具；重新讀取也不會出現。</p>}
      </ReadFailure>
      {data && (
        <>
          <BatchState
            batch={data.batch}
            versionNumber={versionNumber}
            skillId={skillId}
            batchId={batchId}
          />

          <p className="note" data-role="caveat">
            只有判定為符合、部分符合或未符合的試跑才算進通過率；無法判斷或還沒跑完的不算。
          </p>
          <ArmSummary arm={data} heading="載入小工具" />
          {data.skill_withheld && (
            <ArmSummary arm={data.skill_withheld} heading="不載入小工具的對照" />
          )}
          {data.skill_effect && (
            <PairedSummary
              heading="小工具帶來的差別"
              difference={data.skill_effect}
              betterLabel="載入小工具比較常通過"
              worseLabel="不載入反而比較常通過"
            />
          )}
          <TestCaseResults arm={data} names={names} withheld={data.skill_withheld} />
          <RerunLink
            skillId={skillId}
            batchId={batchId}
            batchVersionId={data.batch.skill_version_id}
            batchNumber={versionNumber}
          />
          <BatchComparison skillId={skillId} batchId={batchId} against={against} />
        </>
      )}
    </section>
  );
}

type VersionRow = { version_id: string; version_number: number };

function newestOtherVersion(rows: VersionRow[], batchVersionId: string, batchNumber = 0) {
  const own = batchVersionId.toLowerCase();
  return rows
    .filter((v) => v.version_id.toLowerCase() !== own && v.version_number > batchNumber)
    .reduce<VersionRow | undefined>(
      (best, v) => (best === undefined || v.version_number > best.version_number ? v : best),
      undefined,
    );
}

function RerunLink({
  skillId,
  batchId,
  batchVersionId,
  batchNumber,
}: {
  skillId: string;
  batchId: string;
  batchVersionId: string;
  batchNumber?: number;
}) {
  const versions = useSkillVersions(skillId);
  const newest = newestOtherVersion(
    versions.error ? [] : (versions.data?.versions ?? []),
    batchVersionId,
    batchNumber,
  );
  return (
    <p>
      <Link
        to="/skills/$skillId/evaluation-batches"
        params={{ skillId }}
        search={{ version: newest?.version_id, rerun: batchId }}
      >
        {newest
          ? `用同樣的題目與次數重跑 v${newest.version_number}`
          : "用同樣的題目與次數重跑另一個版本"}
      </Link>
    </p>
  );
}
