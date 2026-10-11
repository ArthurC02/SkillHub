import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { serverReason } from "../../../../core/api/client";

export function MutationError({
  error,
  what,
  fallback,
}: {
  error: unknown;
  what: string;
  fallback: string;
}) {
  return (
    <ReadFailure error={error} what={what}>
      <p role="alert">{serverReason(error) ?? fallback}</p>
    </ReadFailure>
  );
}
