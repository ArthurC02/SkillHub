import { Loading } from "../../../shared/ui/Loading";
import { LoginRequired, ReadFailure } from "../../../shared/ui/LoginRequired";
import { unauthenticated } from "../../../shared/ui/LoginRequired.model";
import { useMe } from "../../../core/session/me.service";
import { useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useRunComparison } from "../evaluation.service";
import { useRun, useRuns } from "../runs.service";
import { ComparisonLead } from "./components/ComparisonLead";
import { ComparisonTables } from "./components/ComparisonTables";
import { CompareCandidatesPicker } from "./components/CompareCandidatesPicker";
import { SkillWorkspaceNav, useSkillVersions } from "../../skill";

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
  const testCaseId = self.data?.test_case_id;
  const siblings = useRuns({ testCaseId, enabled: Boolean(testCaseId) });
  const versions = useSkillVersions(self.data?.skill_id ?? "");
  const candidates = testCaseId
    ? (siblings.data?.pages.flatMap((p) => p.runs) ?? []).filter((r) => r.run_id !== runId)
    : [];

  const pick = (id: string) =>
    void navigate({ to: "/runs/$runId/compare", params: { runId }, search: { against: id } });

  const pickForm = (
    <CompareCandidatesPicker
      selfPending={self.isPending}
      selfError={self.error}
      testCaseId={testCaseId}
      siblingsPending={siblings.isPending}
      siblingsError={siblings.error}
      candidates={candidates}
      hasMoreCandidates={siblings.hasNextPage}
      loadingMoreCandidates={siblings.isFetchingNextPage}
      onLoadMoreCandidates={() => void siblings.fetchNextPage()}
      versionsPending={versions.isPending}
      versionsError={versions.error}
      versions={versions.data?.versions ?? []}
      draft={draft}
      onDraftChange={setDraft}
      onPick={pick}
    />
  );

  return (
    <section>
      <h1>試跑比較</h1>
      {self.data && (
        <SkillWorkspaceNav
          skillId={self.data.skill_id}
          versionId={self.data.skill_version_id}
          testCaseId={self.data.test_case_id}
        />
      )}

      {comparison.data && <ComparisonLead data={comparison.data} />}

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

      {comparison.isPending && against !== "" && <Loading what="比較" />}
      <ReadFailure error={comparison.error} what="比較結果">
        <p role="alert">比較結果暫時讀不到，請稍後再試一次。</p>
      </ReadFailure>
      {comparison.data && <ComparisonTables data={comparison.data} />}

      <p className="note">
        <Link to="/runs/$runId" params={{ runId }}>
          回到這次試跑的詳情
        </Link>
      </p>
    </section>
  );
}
