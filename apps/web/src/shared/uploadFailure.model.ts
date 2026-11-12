import { SessionNotReadyError } from "../core/api/client";

export type FailedUpload = {
  error: unknown;
  fileName?: string;
  blockedError?: SessionNotReadyError;
};

export function readUploadFailure(
  upload: { error: unknown; variables?: { name?: string } },
  previous?: FailedUpload,
): FailedUpload {
  return upload.error instanceof SessionNotReadyError && previous
    ? { ...previous, blockedError: upload.error }
    : { error: upload.error, fileName: upload.variables?.name };
}
