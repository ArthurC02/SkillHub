import { ApiError, isWriteResultUnknown, userFacingReason } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { unconfirmedTestCases } from "../batches.model";
import { UnconfirmedTestCase } from "./UnconfirmedTestCase";

export function CreateOutcome({
  error,
  skillId,
  versionId,
  names,
}: {
  error: unknown;
  skillId: string;
  versionId: string;
  names: Map<string, string>;
}) {
  const unconfirmed =
    error instanceof ApiError && error.status === 422
      ? unconfirmedTestCases(error.body)
      : undefined;
  if (!unconfirmed)
    return (
      <ReadFailure error={error} what="評估批次">
        <p role="alert">
          {isWriteResultUnknown(error)
            ? "不確定批次有沒有開成。先看下面「這個小工具的評估批次」有沒有新的一筆，再決定要不要重送，免得重複扣點。"
            : (userFacingReason(error) ?? "批次沒有開成，請再試一次。")}
        </p>
      </ReadFailure>
    );
  return (
    <section aria-labelledby="unconfirmed-heading">
      <h3 id="unconfirmed-heading" tabIndex={-1}>
        這幾題要先確認試跑權限
      </h3>
      <p role="alert">
        批次還沒開。看過每一題這次試跑可以接觸的範圍並確認後，再按一次「開始評估批次」。
      </p>
      {unconfirmed.map((id) => (
        <UnconfirmedTestCase
          key={id}
          skillId={skillId}
          versionId={versionId}
          testCaseId={id}
          name={names.get(id.toLowerCase())}
        />
      ))}
    </section>
  );
}
