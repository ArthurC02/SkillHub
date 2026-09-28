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
          只顯示 <strong>{rows[0]?.skill_name || ownedSkillName || "某一個 Skill"}</strong> 的 Test
          Case。{" "}
          <Link to="/lab/test-cases" search={{ skill: undefined, version: undefined }}>
            顯示全部
          </Link>
        </p>
      )}
      {notMine && (
        <p className="notice" role="status">
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以這裡看不到它，建立表單的 Skill
          選單也選不到它——
          <Link to="/skills/$skillId" params={{ skillId: filter as string }}>
            先把它 Fork 一份
          </Link>
          ，才會有屬於你的版本可以建立 Test Case。
        </p>
      )}
      {isPending && <Loading what=" Test Case 清單" />}
      <ReadFailure error={error} what=" Test Case" />
      {!isPending &&
        !error &&
        (rows.length === 0 ? (
          notMine ? null : (
            <p>{filter ? "這個 Skill 還沒有 Test Case。" : "還沒有 Test Case。"}</p>
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
                  Skill：
                  {tc.skill_name === ""
                    ? "這個 Skill 已經不在你的清單裡（已刪除，或已下架）"
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
