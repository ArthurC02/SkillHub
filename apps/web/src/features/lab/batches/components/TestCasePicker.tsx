import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import type { useTestCases } from "../../testcases.service";

export function TestCasePicker({
  testCases,
  chosen,
  onToggle,
}: {
  testCases: ReturnType<typeof useTestCases>;
  chosen: string[];
  onToggle: (testCaseId: string, on: boolean) => void;
}) {
  const rows = testCases.error ? [] : (testCases.data?.pages.flatMap((p) => p.test_cases) ?? []);
  return (
    <fieldset>
      <legend>要跑哪些測試題</legend>
      {testCases.isPending && <Loading what="測試題" paused={testCases.fetchStatus === "paused"} />}
      <ReadFailure
        error={testCases.error}
        what="測試題"
        retry={() => void testCases.refetch()}
        retrying={testCases.fetchStatus !== "idle"}
      />
      {testCases.data && !testCases.error && rows.length === 0 && (
        <p role="status">這個小工具還沒有測試題，先建立測試題才能開評估批次。</p>
      )}
      {rows.map((row) => (
        <p key={row.test_case_id}>
          <label>
            <input
              type="checkbox"
              checked={chosen.includes(row.test_case_id)}
              onChange={(e) => onToggle(row.test_case_id, e.target.checked)}
            />{" "}
            {row.name}
          </label>
        </p>
      ))}
      {testCases.hasNextPage && !testCases.error && (
        <button
          type="button"
          disabled={testCases.isFetchingNextPage}
          onClick={() => void testCases.fetchNextPage()}
        >
          {testCases.isFetchingNextPage ? "載入中…" : "載入更多測試題"}
        </button>
      )}
    </fieldset>
  );
}
