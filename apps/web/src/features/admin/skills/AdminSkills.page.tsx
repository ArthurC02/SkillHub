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

function GovernanceResult({
  completed,
  skills,
}: {
  completed?: { id: string; message: string; takedown?: boolean };
  skills?: SkillGovernance[];
}) {
  const result = useRef<HTMLParagraphElement>(null);
  const show = Boolean(completed && skills?.some((skill) => skill.skill_id === completed.id));
  useEffect(() => {
    if (show) {
      result.current?.focus();
      result.current?.scrollIntoView?.({ block: "center" });
    }
  }, [completed, show]);
  if (!show || !completed) return null;
  return (
    <p
      id={completed.takedown ? "admin-takedown-result" : "admin-governance-result"}
      ref={result}
      tabIndex={-1}
      className="notice notice-success"
      role="status"
    >
      {completed.message}
    </p>
  );
}

function GovernanceResults({ q }: { q: string }) {
  const skills = useGovernance(q);
  const readable = q !== "" && !skills.error && !skills.isFetching;
  const found = readable ? (skills.data?.skills ?? []) : [];
  const [completed, setCompleted] = useState<{
    id: string;
    message: string;
    takedown?: boolean;
  }>();

  return (
    <>
      {q === "" && <p className="note">輸入 ID 或名稱，查詢所有工作區的小工具。</p>}
      {q !== "" && skills.isFetching && <Loading what="小工具治理狀態" />}
      <GovernanceResult completed={completed} skills={skills.data?.skills} />
      <ReadFailure error={skills.isFetching ? undefined : skills.error} what="小工具">
        <p role="alert">暫時無法讀取小工具。請重新整理治理狀態後再操作。</p>
      </ReadFailure>
      {q !== "" && (
        <p className="note">
          {skills.data && readable && (
            <>
              治理狀態上次取得於{" "}
              <Timestamp at={new Date(skills.dataUpdatedAt).toISOString()} relative />。{" "}
            </>
          )}
          <button
            type="button"
            aria-disabled={skills.isFetching}
            onClick={() => {
              if (!skills.isFetching) void skills.refetch();
            }}
          >
            {skills.isFetching ? "重新整理中…" : "重新整理治理狀態"}
          </button>
          <br />
          狀態更新時會清除未送出的操作草稿。
        </p>
      )}
      {skills.data &&
        readable &&
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
                  focusWhenTakenDown={
                    completed?.takedown !== true || completed.id !== skill.skill_id
                  }
                />
              ))}
            </ul>
          </>
        ))}
      {found.length === 1 && found[0].takedown_at === null && (
        <GovernanceActions
          key={found[0].skill_id}
          skill={found[0]}
          onTakedown={() =>
            setCompleted({
              id: found[0].skill_id,
              message: `「${found[0].name}」已下架。`,
              takedown: true,
            })
          }
          onOutcome={(message) => setCompleted({ id: found[0].skill_id, message })}
          onDraftChange={() => setCompleted(undefined)}
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
