import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { ApiError } from "../api/client";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Timestamp } from "../components/Timestamp";
import { deleteSkill } from "../api/skills";
import { useOwnSkills } from "../api/testcases";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { useGenerateEntryPoint } from "../api/generate";
import { useCreationEntryPoint } from "../api/creation";
import { CreateHub } from "../components/CreateHub";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { RiskSummary } from "../components/RiskIndicator";
import { FacetNotes, liftedNotes, type FacetNote } from "../components/FacetNotes";
import type { OwnSkill, Redistribution } from "../api/types";

const OWN_SKILL_NOTES: Array<FacetNote<OwnSkill>> = [
  {
    key: "verification",
    label: "掃描狀態",
    note: (s) => s.verification?.note,
    by: (s) => s.verification.label,
  },
];

export const REDISTRIBUTION_BADGE: Record<Redistribution, { text: string; danger?: true }> = {
  allowed: { text: "可打包下載" },
  self_supplied: { text: "可下載（你自己帶進來的）" },
  generated: { text: "可下載（平台為你生成的）" },
  blocked: { text: "不可散布", danger: true },
  unknown: { text: "授權未知，不能打包", danger: true },
};

function RedistributionBadge({ value }: { value: string }) {
  const badge = Object.prototype.hasOwnProperty.call(REDISTRIBUTION_BADGE, value)
    ? REDISTRIBUTION_BADGE[value as Redistribution]
    : REDISTRIBUTION_BADGE.unknown;
  return <span className={badge.danger ? "badge badge-danger" : "badge"}>{badge.text}</span>;
}

export function WorkspaceSkills() {
  const skills = useOwnSkills();
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const generateExposed = useGenerateEntryPoint();
  const creationExposed = useCreationEntryPoint();
  const rows = skills.data?.skills ?? [];
  const hasSkills = rows.length > 0;
  const isEmpty = Boolean(skills.data) && !hasSkills;
  const lifted = liftedNotes(rows, OWN_SKILL_NOTES);

  const remove = useMutation({
    mutationFn: deleteSkill,
    onSuccess: async (result) => {
      setMessage(`已刪除。${result.note}`);
      await client.invalidateQueries({ queryKey: ["own-skills"] });
    },
    onError: () => {},
  });

  return (
    <section>
      <h1>我的 Skill</h1>
      {hasSkills && (
        <p className="note" data-role="teaching">
          Fork 與匯入的都在這裡；公開目錄的不在。
        </p>
      )}

      {isEmpty && <p>你還沒有任何 Skill——這是一份空清單，不是讀取失敗。</p>}
      {/* Two mount points (here and below) rather than one reordered with CSS
          `order`: that only moves the visual position, not the DOM/tab order. */}
      {!hasSkills && (
        <CreateHub generateExposed={generateExposed} creationExposed={creationExposed} />
      )}

      {skills.isPending && <Loading what="你的 Skill 清單" />}
      <ReadFailure error={skills.error} what="你的 Skill 清單" />
      {message && <p role="status">{message}</p>}
      {remove.error && (
        <ReadFailure error={remove.error} what="刪除 Skill">
          <p role="alert">
            {remove.error instanceof ApiError && remove.error.status === 404
              ? "這個 Skill 已經不在了。"
              : "沒有刪成，可以再按一次。"}
          </p>
        </ReadFailure>
      )}

      {hasSkills && (
        <p className="note">
          相容性驗證（Agent 是否載入、Runtime 是否齊備）不在這份清單的資料裡，
          平台目前也不會為你自己的 Skill 量測它。
        </p>
      )}
      {hasSkills && <FacetNotes rows={rows} facets={OWN_SKILL_NOTES} />}

      {hasSkills && (
        <ul className="search-results">
          {rows.map((s) => (
            <li key={s.skill_id} className="search-result">
              <p>
                <Link to="/skills/$skillId" params={{ skillId: s.skill_id }}>
                  <strong>{s.name}</strong>
                </Link>
              </p>
              <p>{s.summary}</p>
              <p className="badge-row">
                <RedistributionBadge value={s.redistribution} />
                {s.access_restriction && (
                  <span className="badge badge-danger">授權保留：{s.access_restriction}</span>
                )}
                {s.forked_from_skill_id ? (
                  <span className="badge">
                    Fork 自
                    <Link to="/skills/$skillId" params={{ skillId: s.forked_from_skill_id }}>
                      來源 Skill
                    </Link>
                  </span>
                ) : (
                  <span className="badge">自己匯入</span>
                )}
                <span
                  className={
                    s.verification.value === "scanned" ? "badge" : "badge badge-unverified"
                  }
                >
                  掃描狀態：{s.verification.label}
                  {s.verification.scanned_at && (
                    <>
                      （<Timestamp at={s.verification.scanned_at} />）
                    </>
                  )}
                </span>
                <RiskSummary risk={s.risk} noteInRow={false} />
              </p>
              {!lifted.verification && <p className="note">{s.verification.note}</p>}
              {s.redistribution === "generated" && <GeneratedNotice skillId={s.skill_id} />}
              <ul className="chip-row skill-actions">
                <li>
                  <Link to="/skills/$skillId/files" params={{ skillId: s.skill_id }}>
                    檔案
                  </Link>
                </li>
                <li>
                  <Link
                    to="/skills/$skillId/package"
                    params={{ skillId: s.skill_id }}
                    search={{ version: undefined }}
                  >
                    打包與下載
                  </Link>
                </li>
                <li>
                  <Link to="/lab/test-cases" search={{ skill: s.skill_id }}>
                    Test Case
                  </Link>
                </li>
                <li>
                  <ConfirmDelete
                    scopeId={`skill-delete-scope-${s.skill_id}`}
                    pending={remove.isPending}
                    onAsk={() => {
                      setMessage("");
                      remove.reset();
                    }}
                    onConfirm={() => remove.mutate(s.skill_id)}
                    scope={
                      <>
                        刪除的是這個 Skill
                        在你工作區裡的存在：它會離開這份清單與搜尋結果，也不能再拿來試跑或打包。
                        版本快照會凍結保留，不隨這次刪除消失，所以誤刪還有救； 別人 Fork
                        過的版本與歷史 Run 引用的內容不受影響——那是他們的溯源鏈，不是你的。
                        已經打包好的下載檔案要另外刪，在下載紀錄那一頁。
                      </>
                    }
                  />
                </li>
              </ul>
            </li>
          ))}
        </ul>
      )}

      {skills.data?.truncated && (
        <p className="notice" role="status">
          這個工作區的 Skill 共 {skills.data.total} 個，上面只列出前 {skills.data.limit} 個。
          目前沒有翻頁，其餘的要用搜尋找。
        </p>
      )}

      {hasSkills && (
        <CreateHub
          generateExposed={generateExposed}
          creationExposed={creationExposed}
          explain={false}
        />
      )}

      <section className="workspace-index">
        <h2>這個工作區的其他頁</h2>
        <ul className="chip-row">
          <li>
            <Link to="/workspace/downloads">下載紀錄</Link>
          </li>
          <li>
            <Link to="/workspace/runs">Run 歷史</Link>
          </li>
          <li>
            <Link to="/workspace/account">帳號</Link>
          </li>
          <li>
            <Link to="/policy">資料保存政策</Link>
          </li>
        </ul>
      </section>
    </section>
  );
}
