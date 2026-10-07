import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useGovernance, type SkillGovernance } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import { GovernanceRow } from "./components/GovernanceRow";
import { GovernanceActions } from "./components/GovernanceActions";

export function AdminSkills() {
  const { q = "" } = useSearch({ from: "/admin/skills" });
  return <SkillSearch key={q} q={q} />;
}

function SkillSearch({ q }: { q: string }) {
  const navigate = useNavigate();
  const [draft, setDraft] = useState(q);
  const skills = useGovernance(q);
  const queryMatches = draft.trim() === q;
  const refreshing = skills.isFetching && !skills.isFetchingNextPage;
  const showResult = queryMatches && !refreshing && (!skills.error || skills.isFetchNextPageError);

  return (
    <AdminPage
      heading="小工具治理"
      lede="範圍是所有工作區，含私人的與已下架的；只顯示治理狀態，不顯示內容。"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const nextQuery = draft.trim();
          if (nextQuery === q && nextQuery !== "") void skills.refetch();
          else void navigate({ to: "/admin/skills", search: { q: nextQuery || undefined } });
        }}
      >
        <div className="field">
          <label htmlFor="admin-skill-q">小工具 ID 或名稱</label>
          <input
            id="admin-skill-q"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <button type="submit" className="action">
          查詢
        </button>
      </form>
      {!queryMatches && q !== "" && <p role="status">查詢條件已變更；按「查詢」顯示新結果。</p>}
      {queryMatches && q !== "" && refreshing && <Loading what="小工具" />}
      {queryMatches && !refreshing && (
        <ReadFailure
          error={skills.isFetchNextPageError ? undefined : skills.error}
          what="小工具"
          onRetry={() => void skills.refetch()}
          retrying={skills.isFetching}
        />
      )}
      {showResult && skills.data && (
        <GovernanceResults
          q={q}
          found={skills.data.pages.flatMap((page) => page.skills)}
          total={skills.data.pages[0]?.total}
          hasNextPage={skills.hasNextPage}
          isFetchingNextPage={skills.isFetchingNextPage}
          nextPageFailed={skills.isFetchNextPageError}
          onMore={() => void skills.fetchNextPage()}
        />
      )}
    </AdminPage>
  );
}

function GovernanceResults({
  q,
  found,
  total,
  hasNextPage,
  isFetchingNextPage,
  nextPageFailed,
  onMore,
}: {
  q: string;
  found: SkillGovernance[];
  total: number | undefined;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  nextPageFailed: boolean;
  onMore: () => void;
}) {
  if (total === undefined || !Number.isSafeInteger(total) || total < found.length) {
    return <p role="alert">無法確認查詢總數。請重新查詢；確認前不提供治理操作。</p>;
  }
  if (found.length === 0) {
    return <p>沒有符合「{q}」的小工具：0 筆。已刪除的小工具不會出現。</p>;
  }
  return (
    <>
      <p role="status">
        已顯示 {found.length} / {total} 筆。
      </p>
      {nextPageFailed && (
        <p role="alert">
          後續小工具暫時無法讀取；目前只顯示已載入的 {found.length}{" "}
          筆，清單不完整。可以重試載入更多。
        </p>
      )}
      <ul className="download-list">
        {found.map((skill) => (
          <GovernanceRow key={skill.skill_id} skill={skill} single={total === 1} />
        ))}
      </ul>
      {hasNextPage && (
        <button type="button" disabled={isFetchingNextPage} onClick={onMore}>
          {isFetchingNextPage ? "載入中…" : nextPageFailed ? "重試載入更多" : "載入更多"}
        </button>
      )}
      {total === 1 && found[0].takedown_at === null && <GovernanceActions skill={found[0]} />}
    </>
  );
}
