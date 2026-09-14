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
  const navigate = useNavigate();
  const [draft, setDraft] = useState(q);
  const skills = useGovernance(q);
  const found = skills.data?.skills ?? [];

  return (
    <AdminPage
      heading="Skill 治理"
      lede="範圍是所有工作區，含私人的與已下架的；只顯示治理狀態，不顯示內容。"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void navigate({ to: "/admin/skills", search: { q: draft.trim() || undefined } });
        }}
      >
        <div className="field">
          <label htmlFor="admin-skill-q">Skill id 或名稱</label>
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
      {q !== "" && skills.isPending && <Loading what=" Skill" />}
      <ReadFailure error={skills.error} what=" Skill" />
      {skills.data &&
        (found.length === 0 ? (
          <p>沒有符合「{q}」的 Skill：0 筆。已刪除的 Skill 不會出現。</p>
        ) : (
          <ul className="download-list">
            {found.map((skill) => (
              <GovernanceRow key={skill.skill_id} skill={skill} single={found.length === 1} />
            ))}
          </ul>
        ))}
      {found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions skill={found[0]} />
      )}
    </AdminPage>
  );
}
