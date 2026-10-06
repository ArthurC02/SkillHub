import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useGovernance } from "../admin.service";
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
  const found = skills.data?.skills ?? [];
  const queryMatches = draft.trim() === q;
  const showResult = queryMatches && !skills.isFetching && !skills.error;

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
      {queryMatches && q !== "" && skills.isFetching && <Loading what="小工具" />}
      {queryMatches && !skills.isFetching && <ReadFailure error={skills.error} what="小工具" />}
      {showResult &&
        skills.data &&
        (found.length === 0 ? (
          <p>沒有符合「{q}」的小工具：0 筆。已刪除的小工具不會出現。</p>
        ) : (
          <ul className="download-list">
            {found.map((skill) => (
              <GovernanceRow key={skill.skill_id} skill={skill} single={found.length === 1} />
            ))}
          </ul>
        ))}
      {showResult && found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions skill={found[0]} />
      )}
    </AdminPage>
  );
}
