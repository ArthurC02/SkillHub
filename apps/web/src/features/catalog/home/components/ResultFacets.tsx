import { LabelledBadge } from "../../../../shared/ui/LabelledBadge";
import { RiskSummary } from "../../../../shared/ui/RiskIndicator";
import type { LiftedNotes } from "./FacetNotes";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import type { PublicSearchResult } from "../../../../core/api/types";
import "./ResultFacets.css";

export function ResultFacets({ hit, lifted }: { hit: PublicSearchResult; lifted: LiftedNotes }) {
  const untested =
    hit.compatibility.capability.value === "unverified" &&
    hit.compatibility.runtime.value === "unverified";

  return (
    <dl className="result-facets">
      <dt>來源層級</dt>
      <dd>
        <LabelledBadge kind="tier" value={hit.tier} noteInRow={!lifted.tier} />
      </dd>

      <dt>類別</dt>
      <dd>
        <LabelledBadge kind="category" value={hit.category} noteInRow={!lifted.category} />
      </dd>

      <dt>相容狀態</dt>
      <dd>
        規格驗證：{hit.compatibility.spec_validation.label}
        {untested && <span className="badge badge-untested">尚未試跑</span>}
        {!lifted.compatibility && <span className="note">{hit.compatibility.note}</span>}
      </dd>

      <dt>依賴</dt>
      <dd>
        {hit.dependencies.length > 0 ? (
          hit.dependencies.join("、")
        ) : (
          <span className="note">未測量——沒有擷取到依賴資訊，不等於沒有依賴。</span>
        )}
      </dd>

      <dt>風險提示</dt>
      <dd>
        <RiskSummary risk={hit.risk} noteInRow={!lifted.risk} />
      </dd>

      <dt>最近驗證時間</dt>
      <dd>
        {hit.verified_at ? (
          <Timestamp at={hit.verified_at} />
        ) : (
          <span className="note">未測量——這個 Skill 還沒有匯入內容可以驗證。</span>
        )}
      </dd>
    </dl>
  );
}
