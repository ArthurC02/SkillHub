import { Link, useSearch } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { MAX_COMPARE, useEmbeddedSkillDetails } from "../../skill";
import { CompatibilityStatus } from "../../../shared/ui/CompatibilityStatus";
import { LabelledBadge } from "../../../shared/ui/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../../../shared/ui/LicenseBadge";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { RiskIndicator } from "../../../shared/ui/RiskIndicator";
import { Timestamp } from "../../../shared/ui/Timestamp";
import type { SkillDetail, SkillTags } from "../../../core/api/types";
import { IN_PROGRESS, type Absence } from "../../../shared/ui/absence";
import "./Compare.page.css";

type TagBucket = keyof SkillTags;

function tagBucket(skill: SkillDetail, bucket: TagBucket): string | undefined {
  return skill.enrichment.tags?.[bucket].join(" ");
}

function tagRenderer(bucket: TagBucket) {
  return (skill: SkillDetail) => (
    <ul className="tag-list">
      {skill.enrichment.tags?.[bucket].map((tag) => (
        <li key={tag} className="badge">
          {tag}
        </li>
      ))}
    </ul>
  );
}

type Absent = Absence | typeof IN_PROGRESS;

type CompareRow = {
  label: string;
  render?: (skill: SkillDetail) => ReactNode;
} & (
  | { signature: (skill: SkillDetail) => string; absent?: never }
  | {
      signature: (skill: SkillDetail) => string | undefined;
      absent: (skill: SkillDetail) => Absent;
    }
);

const modelAbsence = (skill: SkillDetail): Absent =>
  skill.enrichment.status === "pending" ? IN_PROGRESS : "未測量";

const packageScanned = (skill: SkillDetail) => skill.version !== undefined;

const notApplicable = (): Absent => "不適用";

const ROWS: CompareRow[] = [
  {
    label: "套件自述摘要",
    signature: (skill) => skill.summary || undefined,
    absent: notApplicable,
  },
  {
    label: "白話摘要（AI 產生）",
    signature: (skill) =>
      skill.enrichment.status === "enriched" ? skill.enrichment.summary || undefined : undefined,
    absent: modelAbsence,
  },
  {
    label: "可以用來做什麼（AI 產生的任務範例）",
    signature: (skill) =>
      skill.enrichment.status === "enriched"
        ? skill.enrichment.task_examples?.join("\n")
        : undefined,
    absent: modelAbsence,
    render: (skill) => (
      <ul>
        {skill.enrichment.task_examples?.map((example) => (
          <li key={example}>{example}</li>
        ))}
      </ul>
    ),
  },
  {
    label: "限制",
    signature: (skill) => {
      const listed = skill.limitations.map((limit) => `${limit.source}:${limit.text}`).join("\n");
      if (listed) return listed;
      return skill.enrichment.status === "enriched" && packageScanned(skill) ? "" : undefined;
    },
    absent: modelAbsence,
    render: (skill) => (
      <ul>
        {skill.limitations.map((limit) => (
          <li key={limit.text}>
            {limit.text}
            {limit.source === "model" && (
              <>
                <span className="badge badge-source-model">AI 產生</span>
                <span className="note">由模型整理，未經人工核對</span>
              </>
            )}
          </li>
        ))}
      </ul>
    ),
  },
  {
    label: "輸入",
    signature: (skill) => tagBucket(skill, "inputs"),
    render: tagRenderer("inputs"),
    absent: modelAbsence,
  },
  {
    label: "輸出",
    signature: (skill) => tagBucket(skill, "outputs"),
    render: tagRenderer("outputs"),
    absent: modelAbsence,
  },
  {
    label: "依賴",
    signature: (skill) => tagBucket(skill, "dependencies"),
    render: tagRenderer("dependencies"),
    absent: modelAbsence,
  },
  {
    label: "套件宣告可用的工具（權限）",
    signature: (skill) => skill.allowed_tools?.join(" "),
    absent: (skill) => (packageScanned(skill) ? "不適用" : "未測量"),
    render: (skill) => (
      <ul className="tag-list">
        {skill.allowed_tools?.map((tool) => (
          <li key={tool}>
            <code>{tool}</code>
          </li>
        ))}
      </ul>
    ),
  },
  {
    label: "來源層級",
    signature: (skill) => skill.tier.value,
    render: (skill) => <LabelledBadge kind="tier" value={skill.tier} />,
  },
  {
    label: "來源",
    signature: (skill) =>
      skill.source && `${skill.source.trust.value} ${skill.source.url ?? ""}`.trim(),
    absent: notApplicable,
    render: (skill) =>
      skill.source && (
        <>
          <LabelledBadge kind="trust" value={skill.source.trust} />
          {skill.source.url ? (
            <p>
              <a href={skill.source.url} rel="noreferrer noopener">
                {skill.source.url}
              </a>
            </p>
          ) : (
            <p className="note">
              來源網址：
              <span className="compare-unknown">
                {skill.source.trust.value === "generated" ? "不適用" : "未測量"}
              </span>
            </p>
          )}
          {skill.source.source_version && (
            <p className="note">
              版本／Commit：<code>{skill.source.source_version}</code>
            </p>
          )}
        </>
      ),
  },
  {
    label: "License",
    signature: (skill) =>
      `${skill.license.status.value} ${skill.license.expression ?? ""} ${skill.license.source ?? ""}`.trim(),
    render: (skill) => (
      <>
        <LicenseBadge license={skill.license} />
        <LicenseNotes license={skill.license} />
      </>
    ),
  },
  {
    label: "風險揭露",
    signature: (skill) =>
      `${skill.risk.scan_status} ${skill.risk.counts.errors}/${skill.risk.counts.warnings}/${skill.risk.counts.infos}`,
    render: (skill) => <RiskIndicator risk={skill.risk} />,
  },
  {
    label: "相容性（驗證證據）",
    signature: (skill) =>
      `${skill.compatibility.spec_validation.value}/${skill.compatibility.capability.value}/${skill.compatibility.runtime.value}`,
    render: (skill) => <CompatibilityStatus compatibility={skill.compatibility} />,
  },
  {
    label: "版本與時間",
    signature: (skill) =>
      skill.version && `v${skill.version.version_number} ${skill.version.created_at}`,
    absent: notApplicable,
    render: (skill) =>
      skill.version && (
        <>
          <p>v{skill.version.version_number}</p>
          <p className="note">
            建立時間：
            <Timestamp at={skill.version.created_at} />
          </p>
        </>
      ),
  },
];

export function CompareTable({ skills }: { skills: SkillDetail[] }) {
  return (
    <>
      <h2>逐項比較</h2>
      <div className="table-scroll" tabIndex={0}>
        <table className="compare-table">
          <caption>並排比較 {skills.length} 個 Skill 的靜態資料（匯入時記錄與掃描結果）</caption>
          <thead>
            <tr>
              <th scope="col">比較項目</th>
              {skills.map((skill) => (
                <th key={skill.skill_id} scope="col">
                  <Link to="/skills/$skillId" params={{ skillId: skill.skill_id }}>
                    {skill.name}
                  </Link>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {ROWS.map((row) => {
              const signatures = skills.map(row.signature);
              // Sentinel prefix keeps an absence word from matching a genuinely equal value.
              const differs =
                new Set(
                  skills.map((skill, index) => signatures[index] ?? `\u0000${row.absent?.(skill)}`),
                ).size > 1;
              return (
                <tr key={row.label} className={differs ? "compare-differs" : undefined}>
                  <th scope="row">
                    {row.label}
                    {differs && <span className="badge badge-differs">有差異</span>}
                  </th>
                  {skills.map((skill, index) => (
                    <td key={skill.skill_id}>
                      {signatures[index] === undefined ? (
                        <span className="compare-unknown">{row.absent?.(skill)}</span>
                      ) : signatures[index] === "" ? (
                        "0 項"
                      ) : (
                        (row.render?.(skill) ?? signatures[index])
                      )}
                    </td>
                  ))}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </>
  );
}

export function Compare() {
  const { ids } = useSearch({ from: "/compare" });
  const skillIds = [
    ...new Set(
      ids
        .split(",")
        .map((id) => id.trim())
        .filter(Boolean),
    ),
  ].slice(0, MAX_COMPARE);

  const results = useEmbeddedSkillDetails(skillIds);

  const skills = results.flatMap((result) => (result.data ? [result.data] : []));
  const failed = results.filter((result) => result.isError).length;
  const firstError = results.find((result) => result.error)?.error;

  return (
    <section>
      <h1>並排比較</h1>
      <p className="note">
        以下全部來自靜態資料（匯入時記錄與掃描結果），沒有任何一項是試跑出來的。
      </p>

      {skillIds.length < 2 && (
        <p role="status">請從首頁的搜尋結果或目錄選擇 2 到 3 個 Skill 再進行比較。</p>
      )}
      {results.some((result) => result.isLoading) && (
        <p role="status">
          載入中…（{skillIds.length} 個裡讀到 {skills.length} 個）
        </p>
      )}
      {failed > 0 && (
        <>
          <p role="alert">有 {failed} 個 Skill 讀取失敗，未列入下表。</p>
          <ReadFailure error={firstError} what="這些 Skill" />
        </>
      )}

      {skills.length >= 2 && <CompareTable skills={skills} />}

      <p>
        <Link to="/">回到首頁的目錄</Link>
      </p>
    </section>
  );
}
