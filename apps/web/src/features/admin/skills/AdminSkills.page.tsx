import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useGovernance, type SkillGovernance } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { GovernanceRow } from "./components/GovernanceRow";
import { GovernanceActions } from "./components/GovernanceActions";

function SearchForm({ q, onDraftChange }: { q: string; onDraftChange: (draft: string) => void }) {
  const navigate = useNavigate();
  const [draft, setDraft] = useState(q);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        const query = draft.trim();
        setDraft(query);
        onDraftChange(query);
        void navigate({ to: "/admin/skills", search: { q: query || undefined } });
      }}
    >
      <div className="field">
        <label htmlFor="admin-skill-q">小工具 ID 或名稱</label>
        <input
          id="admin-skill-q"
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            onDraftChange(event.target.value);
          }}
        />
      </div>
      <button type="submit" className="action">
        查詢
      </button>
    </form>
  );
}

function TakedownResult({
  completed,
  skills,
}: {
  completed?: { id: string; name: string };
  skills?: SkillGovernance[];
}) {
  const result = useRef<HTMLParagraphElement>(null);
  const show = Boolean(completed && skills?.some((skill) => skill.skill_id === completed.id));
  useEffect(() => {
    if (show) result.current?.focus();
  }, [show]);
  if (!show || !completed) return null;
  return (
    <p
      id="admin-takedown-result"
      ref={result}
      tabIndex={-1}
      className="notice notice-success"
      role="status"
    >
      「{completed.name}」已下架。
    </p>
  );
}

function GovernanceResults({ q }: { q: string }) {
  const skills = useGovernance(q);
  const found = q !== "" && !skills.error ? (skills.data?.skills ?? []) : [];
  const [completedTakedown, setCompletedTakedown] = useState<{ id: string; name: string }>();

  return (
    <>
      {q === "" && <p className="note">輸入 ID 或名稱，查詢所有工作區的小工具。</p>}
      {q !== "" && skills.isPending && <Loading what="小工具" />}
      <TakedownResult completed={completedTakedown} skills={skills.data?.skills} />
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
          <>
            <p className="note" role="status">
              查到 {found.length} 筆小工具。
            </p>
            <ul className="download-list">
              {found.map((skill) => (
                <GovernanceRow
                  key={skill.skill_id}
                  skill={skill}
                  single={found.length === 1}
                  focusWhenTakenDown={completedTakedown?.id !== skill.skill_id}
                />
              ))}
            </ul>
          </>
        ))}
      {found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions
          key={found[0].skill_id}
          skill={found[0]}
          onTakedown={() => setCompletedTakedown({ id: found[0].skill_id, name: found[0].name })}
        />
      )}
    </>
  );
}

export function AdminSkills() {
  const { q = "" } = useSearch({ from: "/admin/skills" });
  const [editing, setEditing] = useState<{ q: string; draft: string }>();
  const queryChanged = editing?.q === q && editing.draft !== q;

  return (
    <AdminPage
      heading="小工具治理"
      lede="範圍是所有工作區，含私人的與已下架的；只顯示治理狀態，不顯示內容。"
    >
      <SearchForm key={q} q={q} onDraftChange={(draft) => setEditing({ q, draft })} />
      {queryChanged ? (
        <p className="note">查詢條件已變更；按「查詢」載入新小工具。</p>
      ) : (
        <GovernanceResults key={`results:${q}`} q={q} />
      )}
    </AdminPage>
  );
}
