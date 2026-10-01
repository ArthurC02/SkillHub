import { Loading } from "../../../../shared/ui/Loading";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import type { TestCaseListItem } from "../../testcases.service";

export function ExistingTestCasesSection({
  filter,
  version,
  ownedSkillName,
  notMine,
  isPending,
  error,
  rows,
  hasNextPage,
  isFetchingNextPage,
  onFetchNextPage,
}: {
  filter: string | undefined;
  version: string | undefined;
  ownedSkillName: string | undefined;
  notMine: boolean;
  isPending: boolean;
  error: Error | null;
  rows: TestCaseListItem[];
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  onFetchNextPage: () => void;
}) {
  return (
    <>
      {filter && (
        <p className="note" role="status">
          只顯示 <strong>{rows[0]?.skill_name || ownedSkillName || "某一個小工具"}</strong> 的 Test
          Case。{" "}
          <Link to="/lab/test-cases" search={{ skill: undefined, version: undefined }}>
            顯示全部
          </Link>
        </p>
      )}
      {notMine && (
        <p className="notice" role="status">
          這個小工具不在你的工作區。測試題屬於工作區，所以這裡看不到它，建立表單的小工具選單也選不到它——
          <Link to="/skills/$skillId" params={{ skillId: filter as string }}>
            先把它複製一份
          </Link>
          ，才會有屬於你的版本可以建立測試題。
        </p>
      )}
      {isPending && <Loading what="測試題清單" />}
      <ReadFailure error={error} what="測試題" />
      {!isPending &&
        !error &&
        (rows.length === 0 ? (
          notMine ? null : (
            <p>{filter ? "這個小工具還沒有測試題。" : "還沒有測試題。"}</p>
          )
        ) : (
          <ul className="search-results" data-role="evidence">
            {rows.map((tc) => (
              <li key={tc.test_case_id} className="search-result">
                <Link
                  to="/lab/test-cases/$testCaseId"
                  params={{ testCaseId: tc.test_case_id }}
                  search={{ version }}
                >
                  {tc.name}
                </Link>
                <p className="note">
                  小工具：
                  {tc.skill_name === ""
                    ? "這個小工具已經不在你的清單裡（已刪除，或已下架）"
                    : tc.skill_name}
                </p>
                <p className="note">
                  驗收條件已確認 {tc.criteria_confirmed}/{tc.criteria_total} 條 · Rubric{" "}
                  {tc.has_rubric ? "有" : "無"} · 最後修改 <Timestamp at={tc.updated_at} />
                </p>
              </li>
            ))}
          </ul>
        ))}
      {hasNextPage && (
        <button type="button" disabled={isFetchingNextPage} onClick={onFetchNextPage}>
          {isFetchingNextPage ? "載入中…" : "載入更多"}
        </button>
      )}
    </>
  );
}
