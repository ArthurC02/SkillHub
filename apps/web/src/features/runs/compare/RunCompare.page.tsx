import { Loading } from "../../../shared/ui/Loading";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { useMe } from "../../../core/session/me.service";
import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import { useRunComparison } from "../evaluation.service";
import { useRun, useRuns } from "../runs.service";
import { ComparisonLead } from "./components/ComparisonLead";
import { ComparisonTables } from "./components/ComparisonTables";
import { CompareCandidatesPicker } from "./components/CompareCandidatesPicker";
import { SkillWorkspaceNav, useSkillVersions } from "../../skill";
import { usePaginationFocus } from "../../../shared/ui/usePaginationFocus";

export function RunCompare() {
  const { runId } = useParams({ from: "/runs/$runId/compare" });
  const { against = "" } = useSearch({ strict: false }) as { against?: string };
  const [draft, setDraft] = useState(against);
  const [trackedAgainst, setTrackedAgainst] = useState(against);
  if (against !== trackedAgainst) {
    setTrackedAgainst(against);
    setDraft(against);
  }
  const navigate = useNavigate();
  const comparison = useRunComparison(runId, against);
  const me = useMe();
  const loggedOut = unauthenticated(me.error);

  const self = useRun(runId);
  const selfFeedback = readFeedback(self);
  const readableSelf = self.error ? undefined : self.data;
  const testCaseId = readableSelf?.test_case_id;
  const siblings = useRuns({ testCaseId, enabled: Boolean(testCaseId) });
  const versions = useSkillVersions(readableSelf?.skill_id ?? "");
  const versionsFeedback = readFeedback(versions);
  const candidates = testCaseId
    ? (siblings.data?.pages.flatMap((p) => p.runs) ?? []).filter(
        (r) => r.run_id.toLowerCase() !== runId.toLowerCase(),
      )
    : [];
  const {
    loadMore,
    bindTarget,
    headingRef,
    row: continuationRun,
  } = usePaginationFocus(siblings, candidates);

  const pick = (id: string) => {
    void navigate({ to: "/runs/$runId/compare", params: { runId }, search: { against: id } });
    document.getElementById("run-comparison-heading")?.focus();
  };

  const pickForm = (
    <CompareCandidatesPicker
      selfPending={selfFeedback.pending}
      selfPaused={self.fetchStatus === "paused"}
      selfError={selfFeedback.error}
      onRetrySelf={() => void self.refetch()}
      testCaseId={testCaseId}
      siblingsPending={siblings.isPending || (siblings.isError && siblings.isFetching)}
      siblingsPaused={siblings.fetchStatus === "paused"}
      siblingsError={siblings.error}
      onRetrySiblings={() => void siblings.refetch()}
      candidates={candidates}
      hasMoreCandidates={siblings.hasNextPage}
      loadingMoreCandidates={siblings.isFetchingNextPage}
      onLoadMoreCandidates={loadMore}
      continuationRef={bindTarget}
      continuationRunId={continuationRun?.run_id}
      versionsPending={versionsFeedback.pending}
      versionsPaused={versions.fetchStatus === "paused"}
      versionsError={versionsFeedback.error}
      onRetryVersions={() => void versions.refetch()}
      versions={versions.error ? [] : (versions.data?.versions ?? [])}
      draft={draft}
      onDraftChange={setDraft}
      onPick={pick}
    />
  );

  return (
    <section>
      <h1 id="run-comparison-heading" tabIndex={-1} ref={headingRef}>
        試跑比較
      </h1>
      {readableSelf && (
        <>
          <SkillWorkspaceNav
            skillId={readableSelf.skill_id}
            versionId={readableSelf.skill_version_id}
            testCaseId={readableSelf.test_case_id}
          />
          <p className="note">
            比較基準：{sourceVersionLabel(readableSelf.skill_version_id, versions)}
          </p>
        </>
      )}

      <p className="note">
        <Link to="/runs/$runId" params={{ runId }} target="_blank" rel="noopener" preload={false}>
          另開比較基準的試跑結果
        </Link>
      </p>

      <ComparisonResult comparison={comparison} against={against}>
        <details>
          <summary>進階資訊（試跑紀錄識別碼）</summary>
          <ul>
            <li>
              這一邊：<code>{runId}</code>
            </li>
            <li>另一邊：{against === "" ? "尚未選擇" : <code>{against}</code>}</li>
          </ul>
        </details>

        {loggedOut ? (
          <LoginRequired what="試跑比較" />
        ) : comparison.data ? (
          <details>
            <summary>換一個要比較的試跑紀錄</summary>
            {pickForm}
          </details>
        ) : (
          pickForm
        )}
      </ComparisonResult>
    </section>
  );
}

function sourceVersionLabel(
  versionId: string | undefined,
  query: ReturnType<typeof useSkillVersions>,
) {
  if (readFeedback(query).pending)
    return query.fetchStatus === "paused" ? "版本編號待連線" : "版本編號載入中";
  const version =
    !query.error && typeof versionId === "string"
      ? query.data?.versions.find(
          (candidate) => candidate.version_id.toLowerCase() === versionId.toLowerCase(),
        )
      : undefined;
  return version ? `v${version.version_number}` : "版本編號未知";
}

function readFeedback(
  query: Pick<ReturnType<typeof useRun>, "isPending" | "isError" | "fetchStatus" | "error">,
) {
  const pending = query.isPending || (query.isError && query.fetchStatus !== "idle");
  return { pending, error: pending ? null : query.error };
}

const UNUSABLE_PAIR =
  "這兩筆試跑紀錄不能比較：另一筆的 ID 格式不對、不存在、不屬於這個工作區，或就是這一筆本身。請換一筆試跑紀錄。";

function unusablePair(error: unknown) {
  return error instanceof ApiError && (error.status === 400 || error.status === 404);
}

function ComparisonResult({
  comparison,
  against,
  children,
}: {
  comparison: ReturnType<typeof useRunComparison>;
  against: string;
  children: ReactNode;
}) {
  if (against === "") return <>{children}</>;
  const paused = comparison.fetchStatus === "paused";
  const busy = comparison.fetchStatus !== "idle";
  const report = comparison.error || busy ? undefined : comparison.data;
  return (
    <>
      {report && <ComparisonLead data={report} />}
      <button
        type="button"
        aria-disabled={busy}
        onClick={() => {
          if (!busy) void comparison.refetch();
        }}
      >
        {paused ? "等待連線…" : busy ? "更新比較中…" : "更新比較結果"}
      </button>
      {busy && <Loading what="比較" paused={paused} />}
      <ReadFailure
        error={busy ? null : comparison.error}
        what="比較結果"
        retry={unusablePair(comparison.error) ? undefined : () => void comparison.refetch()}
        focusTarget="run-comparison-heading"
      >
        {unusablePair(comparison.error) && <p role="alert">{UNUSABLE_PAIR}</p>}
      </ReadFailure>
      {children}
      {report && <ComparisonTables data={report} />}
    </>
  );
}
