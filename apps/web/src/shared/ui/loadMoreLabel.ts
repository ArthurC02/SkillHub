export function loadMoreLabel(fetching: boolean, paused: boolean, idle = "載入更多"): string {
  if (paused) return "等待連線…";
  return fetching ? "載入中…" : idle;
}
