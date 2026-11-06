import { useId } from "react";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { usePreflight } from "../../lab.service";
import { PreflightFacts } from "../../components/PreflightFacts";
import { runCeilingCredits } from "../../preflight/preflight.model";
import { useConfirmRunPermissions } from "../batches.service";

export function UnconfirmedTestCase({
  skillId,
  versionId,
  testCaseId,
  name,
}: {
  skillId: string;
  versionId: string;
  testCaseId: string;
  name: string | undefined;
}) {
  const headingId = useId();
  const preflight = usePreflight(skillId, versionId, testCaseId, true);
  const confirm = useConfirmRunPermissions(skillId, versionId, testCaseId);
  const facts = preflight.error ? undefined : preflight.data;

  return (
    <section aria-labelledby={headingId}>
      <h4 id={headingId} tabIndex={-1}>
        {name ?? "未載入名稱的測試題"}
      </h4>
      {preflight.isPending && (
        <Loading what="權限摘要" paused={preflight.fetchStatus === "paused"} />
      )}
      <ReadFailure
        error={preflight.error}
        what="權限摘要"
        retry={() => void preflight.refetch()}
        retrying={preflight.fetchStatus !== "idle"}
        focusTarget={headingId}
      />
      {facts && (
        <>
          <PreflightFacts
            summary={facts.summary}
            cost={facts.estimated_cost}
            ceiling={runCeilingCredits(facts.ceiling_credits)}
            quota={facts.quota}
          />
          <ReadFailure error={confirm.error} what="權限確認">
            <p role="alert">這一題的權限還沒確認成功，請再按一次。</p>
          </ReadFailure>
          {confirm.isSuccess ? (
            <p role="status">已確認這一題的試跑權限。</p>
          ) : (
            <button
              type="button"
              disabled={confirm.isPending}
              onClick={() => confirm.mutate(facts.summary_hash)}
            >
              {confirm.isPending ? "確認中…" : "我確認這一題的試跑權限"}
            </button>
          )}
        </>
      )}
    </section>
  );
}
