import { Link } from "@tanstack/react-router";
import type { LiftedNotes } from "./FacetNotes";
import type { PublicSearchResult } from "../../../../core/api/types";
import { ResultFacets } from "./ResultFacets";
import "./SearchResultRow.css";

export function SearchResultRow({
  hit,
  checked,
  atLimit,
  onToggle,
  rankNoteInList = true,
  lifted = {},
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
  lifted?: LiftedNotes;
  rankNoteInList?: boolean;
}) {
  return (
    <li className="search-result">
      <Link to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        {hit.name}
      </Link>
      <label className="compare-pick">
        <input
          type="checkbox"
          checked={checked}
          disabled={!checked && atLimit}
          aria-describedby={!checked && atLimit ? "compare-limit" : undefined}
          onChange={() => onToggle(hit.skill_id)}
        />
        加入比較
      </label>
      <p>
        {hit.summary}{" "}
        {hit.summary_source === "model" && (
          <span
            className="badge badge-source-model"
            title="這段摘要由模型改寫，不是套件作者寫的；你的 Agent 讀的是套件自己的 description"
          >
            AI 改寫
          </span>
        )}
        {hit.summary_source === "package" && (
          <span className="badge badge-source-package" title="套件自己的 frontmatter description">
            作者原文
          </span>
        )}
        {hit.summary_source !== "model" && hit.summary_source !== "package" && (
          <span className="badge badge-source-unknown" title="伺服器沒有回報這段摘要的來源">
            來源未標示
          </span>
        )}
      </p>
      {hit.match_reason && (
        <p className="match-reason">
          符合原因：{hit.match_reason}
          {hit.match_reason_source === "model" && (
            <span className="badge badge-source-model" title="這段說明由模型產生，未經人工核對">
              AI 產生
            </span>
          )}
          {hit.match_reason_source === "template" && (
            <span className="badge badge-source-template" title="依查詢與文件的關鍵字重疊組出">
              規則產生
            </span>
          )}
        </p>
      )}
      <ResultFacets hit={hit} lifted={lifted} />
      {(hit.rank !== null || rankNoteInList) && (
        <p className="rank">
          {hit.rank === null
            ? (hit.rank_note ?? "未計算語意相似度。")
            : `相似度 ${hit.rank.toFixed(2)}`}
        </p>
      )}
    </li>
  );
}
