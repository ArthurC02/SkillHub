import { QueryClient } from "@tanstack/react-query";
import { ApiError, SessionNotReadyError, setSessionReadinessCheck } from "./client";
import { queryKeys } from "./queryKeys";
import type { Me } from "./types";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    },
  },
});

setSessionReadinessCheck(() => {
  const session = queryClient.getQueryState<Me>(queryKeys.me);
  if (!session) return;
  const anonymous =
    session.data === undefined && session.error instanceof ApiError && session.error.status === 401;
  if (session.fetchStatus !== "idle" || (session.status !== "success" && !anonymous)) {
    throw new SessionNotReadyError();
  }
});
