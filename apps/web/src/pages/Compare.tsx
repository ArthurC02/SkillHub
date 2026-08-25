import { Link, useSearch } from "@tanstack/react-router";
import { useQueries } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { embeddedSkillKey, getEmbeddedSkillDetail } from "../api/skills";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import type { SkillDetail, SkillTags } from "../api/types";

/**
 * DISC-009 (spec 02:DISC-004): side-by-side static comparison of 2–3 candidates.
 *
 * Purely a composition of the existing GET /api/skills/{id} — no comparison
 * endpoint. registry/diff.go is deliberately not reusable here: it compares two
 * versions *of one skill* and rejects a cross-skill pair by design (WS-003).
 *
 * The one rule that shapes every row: a field the API did not supply renders as
 * 未知, never as an absence the reader can mistake for a pass
 * (02:DISC-004 缺少資料的欄位顯示未知，不得自行推定為通過). That is why each row
 * declares a `signature` returning `undefined` for "no data" instead of an
 * empty string — empty string is a value, undefined is the lack of one.
 */

/** Ceiling on side-by-side columns; the table stops being readable past this. */
export const MAX_COMPARE = 3;

/**
 * DISC-003 keeps inputs, outputs, tools and dependencies in separate buckets
 * precisely so they can be compared separately; an empty bucket is 未知 because
 * enrichment not having named an input is not the same as there being none.
 */
type TagBucket = keyof SkillTags;

function tagBucket(skill: SkillDetail, bucket: TagBucket): string | undefined {
  return skill.enrichment.tags?.[bucket].join(" ") || undefined;
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

type CompareRow = {
  label: string;
  /**
   * The comparable form of this row's value, used both for the difference
   * highlight and as the default cell text. `undefined` = this skill has no
   * data for the row, which renders as 未知.
   */
  signature: (skill: SkillDetail) => string | undefined;
  /** Richer cell rendering. Only called when `signature` is defined. */
  render?: (skill: SkillDetail) => ReactNode;
};

const ROWS: CompareRow[] = [
  {
    label: "套件自述摘要",
    signature: (skill) => skill.summary || undefined,
  },
  {
    label: "白話摘要（AI 產生）",
    // `pending` is not "no summary exists somewhere" — it is a recorded state,
    // and the note the server wrote for it says which.
    signature: (skill) =>
      skill.enrichment.status === "enriched" ? skill.enrichment.summary : undefined,
  },
  {
    label: "可以用來做什麼（AI 產生的任務範例）",
    signature: (skill) => skill.enrichment.task_examples?.join("\n") || undefined,
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
    // Empty means neither the enrichment nor the scan stated a limit — not that
    // the skill has none, so it reads 未知 rather than an encouraging blank.
    signature: (skill) =>
      skill.limitations.map((limit) => `${limit.source}:${limit.text}`).join("\n") || undefined,
    render: (skill) => (
      <ul>
        {skill.limitations.map((limit) => (
          <li key={limit.text}>
            {limit.text}
            {limit.source === "model" && (
              <span className="badge badge-source-model" title="由模型整理，未經人工核對">
                AI 產生
              </span>
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
  },
  {
    label: "輸出",
    signature: (skill) => tagBucket(skill, "outputs"),
    render: tagRenderer("outputs"),
  },
  {
    label: "依賴",
    signature: (skill) => tagBucket(skill, "dependencies"),
    render: tagRenderer("dependencies"),
  },
  {
    label: "套件宣告可用的工具（權限）",
    signature: (skill) => skill.allowed_tools?.join(" ") || undefined,
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
    // 來源層級, not 類別: this renders `tier` (curated/indexed/external), which
    // is how much review a skill has had. The 類別 the PDM-001 boundary rule
    // defines (data / documents / writing) is a curation judgement the platform
    // does not store at all — labelling the tier as it would put a value under a
    // heading that means something else entirely.
    label: "來源層級",
    signature: (skill) => skill.tier.value,
    render: (skill) => <LabelledBadge kind="tier" value={skill.tier} />,
  },
  {
    label: "來源",
    // Both axes of provenance: how far the source was traced (trust) and where
    // it came from (url). A skill with no source record at all is 未知.
    signature: (skill) =>
      skill.source && `${skill.source.trust.value} ${skill.source.url ?? ""}`.trim(),
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
            /* 設計 §2.1 給這一頁的詞是「未知」，而這一列的其他缺席都已經用
               `.compare-unknown` 印它。「沒有記錄來源網址」是這一頁上唯一一句
               自己發明的缺席措辭。 */
            <p className="note">
              來源網址：<span className="compare-unknown">未知</span>
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
    // ADR-021 two axes plus the status label; two skills both reading "MIT" but
    // from different provenance tiers are a difference worth highlighting.
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
    // scan_status stays inside the signature: "we could not read the package"
    // and "we read it and found nothing" must not compare as equal.
    signature: (skill) =>
      `${skill.risk.scan_status} ${skill.risk.counts.errors}/${skill.risk.counts.warnings}/${skill.risk.counts.infos}`,
    render: (skill) => <RiskIndicator risk={skill.risk} />,
  },
  {
    label: "相容性（驗證證據）",
    signature: (skill) =>
      `${skill.compatibility.spec_validation}/${skill.compatibility.capability}/${skill.compatibility.runtime}`,
    render: (skill) => <CompatibilityStatus compatibility={skill.compatibility} />,
  },
  {
    label: "版本與時間",
    signature: (skill) =>
      skill.version && `v${skill.version.version_number} ${skill.version.created_at}`,
    render: (skill) =>
      skill.version && (
        <>
          <p>v{skill.version.version_number}</p>
          <p className="note">建立時間：{skill.version.created_at}</p>
        </>
      ),
  },
];

/** The table itself, taking already-loaded skills. */
function CompareTable({ skills }: { skills: SkillDetail[] }) {
  return (
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
            // 未知 is its own value here: one skill declaring MIT and another
            // declaring nothing is a difference, and the sentinel keeps it from
            // colliding with a genuine empty-ish signature.
            const differs = new Set(signatures.map((value) => value ?? "\u0000未知")).size > 1;
            return (
              <tr key={row.label} className={differs ? "compare-differs" : undefined}>
                <th scope="row">
                  {row.label}
                  {differs && <span className="badge badge-differs">有差異</span>}
                </th>
                {skills.map((skill, index) => (
                  <td key={skill.skill_id}>
                    {signatures[index] === undefined ? (
                      <span className="compare-unknown">未知</span>
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
  );
}

export function Compare() {
  const { ids } = useSearch({ from: "/compare" });
  const skillIds = ids
    .split(",")
    .map((id) => id.trim())
    .filter(Boolean)
    .slice(0, MAX_COMPARE);

  // useQueries, not a loop of a hook: the hook count has to stay stable and the
  // id list changes with the URL.
  //
  // The embedded key, not the detail page's. Sharing it meant a comparison of
  // three skills wrote three skill_detail_viewed events from a table where no
  // detail page was opened — 01 §11.2's first segment counts sessions that opened
  // a skill, and this was answering yes for sessions that had not. The cache
  // sharing that used to buy is gone, and one extra request is the price
  // (adversarial review, 2026-08-24).
  const results = useQueries({
    queries: skillIds.map((id) => ({
      queryKey: embeddedSkillKey(id),
      queryFn: () => getEmbeddedSkillDetail(id),
      // Same as every hook in api/ — and as of 2026-08-25 that sentence is
      // finally true: it was written here while `api/skills.ts` (five hooks) and
      // `api/trace.ts` still had none, so the line asserted a convention it was
      // the only member of. 資訊架構 IA-6 closed the other six.
      //
      // The default three retries keep `fetchStatus` at "fetching" through the
      // whole 1s+2s+4s backoff, so a read that had already failed went on
      // rendering 「載入中…（2 個裡讀到 0 個）」 for about seven seconds before the
      // page admitted it — a progress claim with nothing behind it, which is the
      // same 設計 §2.1 rule as 未知不是空白. Retrying buys nothing here anyway:
      // this is a plain GET of a skill the reader picked off a list.
      retry: false,
    })),
  });

  const skills = results.flatMap((result) => (result.data ? [result.data] : []));
  const failed = results.filter((result) => result.isError).length;

  return (
    <section>
      <h1>並排比較</h1>
      <p className="note">
        以下全部來自靜態資料（匯入時記錄與掃描結果），沒有任何一項是試跑出來的。
      </p>

      {skillIds.length < 2 && <p role="alert">請從搜尋結果選擇 2 到 3 個 Skill 再進行比較。</p>}
      {/*
        The count, because it is already in hand and it changes: three parallel
        skill reads resolve one at a time, and a bare 「載入中…」 says the same
        thing whether two of the three have already arrived or none has.
      */}
      {results.some((result) => result.isLoading) && (
        <p role="status">
          載入中…（{skillIds.length} 個裡讀到 {skills.length} 個）
        </p>
      )}
      {failed > 0 && <p role="alert">有 {failed} 個 Skill 讀取失敗，未列入下表。</p>}

      {skills.length >= 2 && <CompareTable skills={skills} />}

      <p>
        <Link to="/">回到搜尋</Link>
      </p>
    </section>
  );
}
