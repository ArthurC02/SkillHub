import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useGovernance } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { GovernanceRow } from "./components/GovernanceRow";
import { GovernanceActions } from "./components/GovernanceActions";

function SearchForm({ q }: { q: string }) {
  const navigate = useNavigate();
  const [draft, setDraft] = useState(q);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void navigate({ to: "/admin/skills", search: { q: draft.trim() || undefined } });
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
  );
}

export function AdminSkills() {
  const { q = "" } = useSearch({ from: "/admin/skills" });
  const skills = useGovernance(q);
  const found = q !== "" && !skills.error ? (skills.data?.skills ?? []) : [];

  return (
    <AdminPage
      heading="小工具治理"
      lede="範圍是所有工作區，含私人的與已下架的；只顯示治理狀態，不顯示內容。"
    >
      <SearchForm key={q} q={q} />
      {q === "" && <p className="note">輸入 ID 或名稱，查詢所有工作區的小工具。</p>}
      {q !== "" && skills.isPending && <Loading what="小工具" />}
      <ReadFailure error={skills.error} what="小工具" />
      {q !== "" && (
        <p className="note">
          {skills.data && !skills.error && (
            <>
              治理狀態上次取得於{" "}
              <Timestamp at={new Date(skills.dataUpdatedAt).toISOString()} relative />。{" "}
            </>
          )}
          <button type="button" disabled={skills.isFetching} onClick={() => void skills.refetch()}>
            {skills.isFetching ? "重新整理中…" : "重新整理治理狀態"}
          </button>
        </p>
      )}
      {q !== "" &&
        skills.data &&
        !skills.error &&
        (found.length === 0 ? (
          <p>沒有符合「{q}」的小工具：0 筆。已刪除的小工具不會出現。</p>
        ) : (
          <ul className="download-list">
            {found.map((skill) => (
              <GovernanceRow key={skill.skill_id} skill={skill} single={found.length === 1} />
            ))}
          </ul>
        ))}
      {found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions key={found[0].skill_id} skill={found[0]} />
      )}
    </AdminPage>
  );
}
