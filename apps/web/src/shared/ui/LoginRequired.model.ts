import { ApiError, userFacingReason } from "../../core/api/client";

export function unauthenticated(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}

export function rateLimitedSentence(error: unknown): string | undefined {
  if (!(error instanceof ApiError) || error.status !== 429) return undefined;
  return userFacingReason(error) ?? "請求太頻繁了，請稍等一下再試。";
}
