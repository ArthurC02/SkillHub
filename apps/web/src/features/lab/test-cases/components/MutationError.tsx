import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { ApiError } from "../../../../core/api/client";

export function MutationError({
  error,
  what,
  fallback,
  serverSaysStatuses = [],
}: {
  error: unknown;
  what: string;
  fallback: string;
  serverSaysStatuses?: number[];
}) {
  return (
    <ReadFailure error={error} what={what}>
      <p role="alert">
        {error instanceof ApiError && serverSaysStatuses.includes(error.status)
          ? error.message
          : fallback}
      </p>
    </ReadFailure>
  );
}
