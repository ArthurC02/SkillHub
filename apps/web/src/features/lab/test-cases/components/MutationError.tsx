import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { isWriteResultUnknown, serverReason } from "../../../../core/api/client";

export function MutationError({
  error,
  what,
  fallback,
  unknown,
}: {
  error: unknown;
  what: string;
  fallback: string;
  unknown?: string;
}) {
  return (
    <ReadFailure error={error} what={what}>
      <p role="alert">
        {isWriteResultUnknown(error)
          ? (unknown ?? "操作結果尚未確認；請先確認最新資料，再決定是否重送。")
          : (serverReason(error) ?? fallback)}
      </p>
    </ReadFailure>
  );
}
