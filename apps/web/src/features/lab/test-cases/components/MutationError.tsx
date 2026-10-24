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
        {(unknown && isWriteResultUnknown(error) ? unknown : serverReason(error)) ?? fallback}
      </p>
    </ReadFailure>
  );
}
