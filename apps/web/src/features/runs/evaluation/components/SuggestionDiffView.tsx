import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useSuggestionDiff } from "../../evaluation.service";
import { Reveal } from "../../../../shared/ui/Reveal";
import { BLOCKED_REASON_LABEL } from "../evaluation.model";

export function SuggestionDiffView({ suggestionId }: { suggestionId: string }) {
  const diff = useSuggestionDiff(suggestionId, true);
  const id = `suggestion-diff-${suggestionId}`;

  return (
    <section id={id} tabIndex={-1} aria-label="建議差異">
      {diff.isPending ? (
        <Loading what="差異" />
      ) : diff.error ? (
        <ReadFailure
          error={diff.error}
          what="差異"
          retry={() => {
            void diff.refetch();
            const section = document.getElementById(id);
            section?.focus();
            section?.scrollIntoView({ block: "center" });
          }}
          retrying={diff.isFetching}
        />
      ) : (
        <>
          {!diff.data.applicable && (
            <p className="notice notice-danger">
              目前無法套用：
              {diff.data.blocked_reason
                ? BLOCKED_REASON_LABEL[diff.data.blocked_reason]
                : "伺服器沒有給原因。"}
            </p>
          )}
          {diff.data.unified_diff ? (
            <pre className="diff">
              <Reveal text={diff.data.unified_diff} />
            </pre>
          ) : (
            <p>沒有可顯示的差異。</p>
          )}
        </>
      )}
    </section>
  );
}
