import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { ApiError } from "../api/client";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { deleteSkill } from "../api/skills";
import { useOwnSkills } from "../api/testcases";
import { ConfirmDelete } from "../components/ConfirmDelete";
import { useGenerateEntryPoint } from "../api/generate";
import { useCreationEntryPoint } from "../api/creation";
import { CreateHub } from "../components/CreateHub";
import { StateIcon } from "../components/StateIcon";
import { followPointer, releasePointer } from "../components/spotlight";
import type { OwnSkill, Redistribution } from "../api/types";

export const REDISTRIBUTION_BADGE: Record<Redistribution, { text: string; danger?: true }> = {
  allowed: { text: "可打包下載" },
  self_supplied: { text: "可下載（你自己帶進來的）" },
  generated: { text: "可下載（平台為你生成的）" },
  blocked: { text: "不可散布", danger: true },
  unknown: { text: "授權未知，不能打包", danger: true },
};

function redistributionOf(value: string) {
  return Object.prototype.hasOwnProperty.call(REDISTRIBUTION_BADGE, value)
    ? REDISTRIBUTION_BADGE[value as Redistribution]
    : REDISTRIBUTION_BADGE.unknown;
}

const TILE_TONES = 4;

function toneOf(skillId: string) {
  return [...skillId].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7) % TILE_TONES;
}

function initialOf(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?";
}

function SkillFlags({ skill }: { skill: OwnSkill }) {
  const redistribution = redistributionOf(skill.redistribution);
  const scanned = skill.risk.scan_status === "scanned";
  const flags = [
    redistribution.danger && (
      <span key="redistribution" className="badge badge-danger">
        {redistribution.text}
      </span>
    ),
    skill.access_restriction && (
      <span key="restriction" className="badge badge-danger">
        授權保留：{skill.access_restriction}
      </span>
    ),
    skill.redistribution === "generated" && (
      <span key="generated" className="badge badge-unverified">
        平台生成，未經人工檢視
      </span>
    ),
    scanned && skill.risk.warnings > 0 && (
      <span key="warnings" className="badge badge-risk">
        <StateIcon state="fail" />
        警告 {skill.risk.warnings}
      </span>
    ),
    ...(scanned ? skill.risk.disclosures : []).map((d) => (
      <span key={d.code} className="badge badge-risk-flag">
        <StateIcon state="fail" />
        {d.label}
      </span>
    )),
  ].filter(Boolean);
  if (flags.length === 0) return null;
  return <p className="badge-row skill-card-flags">{flags}</p>;
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
        <ul
          className="search-results skill-grid"
          onPointerMove={followPointer}
          onPointerLeave={releasePointer}
        >
          {rows.map((s) => (
            <li
              key={s.skill_id}
              className="search-result skill-card"
              data-tone={toneOf(s.skill_id)}
            >
              <Link
                className="skill-card-link"
                to="/skills/$skillId"
                params={{ skillId: s.skill_id }}
              >
                <span className="skill-cover" aria-hidden="true" />
                <span className="skill-card-head">
                  <span className="skill-mono" aria-hidden="true">
                    {initialOf(s.name)}
                  </span>
                  <strong className="skill-card-name">{s.name}</strong>
                </span>
                <span className="skill-card-summary">{s.summary}</span>
              </Link>
              <SkillFlags skill={s} />
              <details className="skill-menu">
                <summary aria-label={`管理「${s.name}」`}>
                  管理 <span aria-hidden="true">▾</span>
                </summary>
                <ul className="skill-menu-list">
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
                  {s.forked_from_skill_id && (
                    <li>
                      <Link to="/skills/$skillId" params={{ skillId: s.forked_from_skill_id }}>
                        Fork 來源 Skill
                      </Link>
                    </li>
                  )}
                  <li className="skill-menu-delete">
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
              </details>
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
