import { ApiError } from "../../../core/api/client";
import { ReadFailure } from "../../../shared/ui/LoginRequired";

export function RecordReadFailure({
  query,
  what,
  focusTarget,
}: {
  query: { error: unknown; isFetching: boolean; refetch: () => unknown };
  what: string;
  focusTarget: string;
}) {
  const gone =
    query.error instanceof ApiError && (query.error.status === 400 || query.error.status === 404);
  return (
    <ReadFailure
      error={query.error}
      what={what}
      retry={gone ? undefined : () => void query.refetch()}
      retrying={query.isFetching}
      focusTarget={focusTarget}
    >
      {gone && <p role="alert">{`找不到${what}，重新讀取也不會出現。`}</p>}
    </ReadFailure>
  );
}
