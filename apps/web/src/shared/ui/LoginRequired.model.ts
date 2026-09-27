import { ApiError } from "../../core/api/client";

export function unauthenticated(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}
