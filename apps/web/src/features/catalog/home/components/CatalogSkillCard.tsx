import { Link } from "@tanstack/react-router";
import type { PublicSearchResult } from "../../../../core/api/types";
import { LabelledBadge } from "../../../../shared/ui/LabelledBadge";
import { StateIcon } from "../../../../shared/ui/StateIcon";
import "./CatalogSkillCard.css";

function SummarySource({ source }: { source: PublicSearchResult["summary_source"] }) {
  if (source === "model") return <span className="badge badge-source-model">AI 改寫</span>;
  if (source === "package") return <span className="badge badge-source-package">作者原文</span>;
  return <span className="badge badge-source-unknown">來源未標示</span>;
}

function CompactRisk({ hit }: { hit: PublicSearchResult }) {
  if (hit.risk.scan_status !== "scanned" || hit.risk.level === "unknown") {
    return (
      <span className="badge badge-unverified">
        <StateIcon state="unknown" />
        未掃描
      </span>
    );
  }

  const disclosures = hit.risk.disclosures.length;
  if (hit.risk.warnings > 0 || disclosures > 0) {
    return (
      <span className="badge badge-risk">
        <StateIcon state="fail" />
        {hit.risk.warnings > 0 ? `警告 ${hit.risk.warnings}` : `風險揭露 ${disclosures}`}
      </span>
    );
  }

  return <span className="badge">掃描未見警告 ≠ 安全</span>;
}

function dependencySummary(dependencies: string[]) {
  if (dependencies.length === 0) return "未提供";
  if (dependencies.length <= 2) return dependencies.join("、");
  return `${dependencies.slice(0, 2).join("、")} ＋${dependencies.length - 2} 項`;
}

function VerifiedDate({ at }: { at: string }) {
  const matched = /^(\d{4})-(\d{2})-(\d{2})/.exec(at);
  return <time dateTime={at}>{matched ? matched.slice(1).join("/") : at}</time>;
}

export function CatalogSkillCard({
  hit,
  checked,
  atLimit,
  onToggle,
}: {
  hit: PublicSearchResult;
  checked: boolean;
  atLimit: boolean;
  onToggle: (skillId: string) => void;
}) {
  const untested =
    hit.compatibility.capability.value === "unverified" &&
    hit.compatibility.runtime.value === "unverified";

  return (
    <li className="catalog-skill-card" data-category={hit.category.value}>
      <div className="catalog-card-header">
        <div className="catalog-card-facts">
          <LabelledBadge kind="tier" value={hit.tier} noteInRow={false} />
          <LabelledBadge kind="category" value={hit.category} noteInRow={false} />
        </div>
        <label className="catalog-card-compare">
          <input
            type="checkbox"
            checked={checked}
            disabled={!checked && atLimit}
            aria-describedby={!checked && atLimit ? "compare-limit" : undefined}
            onChange={() => onToggle(hit.skill_id)}
          />
          比較
        </label>
      </div>

      <Link className="catalog-card-title" to="/skills/$skillId" params={{ skillId: hit.skill_id }}>
        <h3>{hit.name}</h3>
      </Link>

      <p className="catalog-card-summary">
        {hit.summary} <SummarySource source={hit.summary_source} />
      </p>

      <div className="catalog-card-signals">
        <span className="catalog-card-validation">
          規格驗證：{hit.compatibility.spec_validation.label}
          {untested && " · 尚未試跑"}
        </span>
        <CompactRisk hit={hit} />
      </div>

      <dl className="catalog-card-metadata">
        <div>
          <dt>依賴</dt>
          <dd>{dependencySummary(hit.dependencies)}</dd>
        </div>
        <div>
          <dt>最近驗證</dt>
          <dd>{hit.verified_at ? <VerifiedDate at={hit.verified_at} /> : "未測量"}</dd>
        </div>
      </dl>
    </li>
  );
}
