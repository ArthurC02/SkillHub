import { useRef, useState } from "react";
import { useContinuationFocus } from "./useContinuationFocus";

interface PaginationQuery {
  data?: { pages: readonly unknown[] };
  fetchStatus: "idle" | "fetching" | "paused";
  error: unknown;
  hasNextPage: boolean;
  fetchNextPage: () => unknown;
}

export function usePaginationFocus<T>(query: PaginationQuery, items: readonly T[]) {
  const [request, setRequest] = useState({ sequence: 0, pageCount: 0, itemCount: 0 });
  const target = useRef<HTMLElement>(null);
  const available =
    request.sequence > 0 &&
    query.fetchStatus === "idle" &&
    Boolean(query.error || (query.data?.pages.length ?? 0) > request.pageCount);
  useContinuationFocus(request.sequence ? String(request.sequence) : undefined, available, target);
  const row =
    request.sequence > 0 && !query.hasNextPage && !query.error
      ? (items[request.itemCount] ?? items.at(-1))
      : undefined;
  const heading = Boolean(query.error || (available && !query.hasNextPage && row === undefined));
  const bindTarget = (element: HTMLElement | null) => {
    target.current = element;
  };
  return {
    loadMore: () => {
      setRequest((previous) => ({
        sequence: previous.sequence + 1,
        pageCount: query.data?.pages.length ?? 0,
        itemCount: items.length,
      }));
      void query.fetchNextPage();
    },
    bindTarget,
    headingRef: heading ? bindTarget : undefined,
    row,
  };
}
