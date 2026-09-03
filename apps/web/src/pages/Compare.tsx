import { Link, useSearch } from "@tanstack/react-router";
import { useQueries } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { embeddedSkillKey, getEmbeddedSkillDetail } from "../api/skills";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { ReadFailure } from "../components/LoginRequired";
import { RiskIndicator } from "../components/RiskIndicator";
import { Timestamp } from "../components/Timestamp";
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
  /** The row-specific meaning of absent data; absence is not always a measurement. */
  absent?: (skill: SkillDetail) => ReactNode;
};

const enrichmentAbsence = (skill: SkillDetail) =>
  skill.enrichment.status === "pending" ? "處理中" : "未提供";

const tagAbsence = (skill: SkillDetail) =>
  skill.enrichment.status === "pending" ? "處理中" : "未測量";

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
    absent: enrichmentAbsence,
  },
  {
    label: "可以用來做什麼（AI 產生的任務範例）",
    signature: (skill) => skill.enrichment.task_examples?.join("\n") || undefined,
    absent: enrichmentAbsence,
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
              <>
                <span className="badge badge-source-model" title="由模型整理，未經人工核對">
                  AI 產生
                </span>
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
    absent: tagAbsence,
  },
  {
    label: "輸出",
    signature: (skill) => tagBucket(skill, "outputs"),
    render: tagRenderer("outputs"),
    absent: tagAbsence,
  },
  {
    label: "依賴",
    signature: (skill) => tagBucket(skill, "dependencies"),
    render: tagRenderer("dependencies"),
    absent: tagAbsence,
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
            /* 設計 §2.9: 「未知」不在那張封閉的六詞表上。§2.1 本文自己寫著
               「缺席有型別，見 §2.9。只寫『未知』已經不夠了」，而這一頁的每一格
               缺席都是這個詞——它比 §2.9 早。表上對得起這一格的是「未測量」：
               平台有這個欄位，只是這一份沒有記到值。

               §2.9 真正要的另一半還沒做：**型別要是資料層的列舉，不是渲染層的
               falsy**，而下面那張表用一個 `undefined` 服務十四列不同的缺席。逐列
               宣告自己是哪一型，記在 04 丙-131。 */
            <p className="note">
              來源網址：<span className="compare-unknown">未提供</span>
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
    // `.value` on all three, and that is the whole of this row's history: the
    // three axes are `Labelled` objects, so interpolating them produced
    // "[object Object]/[object Object]/[object Object]" — the same string for
    // every skill on the table. `differs` was therefore permanently false, and
    // the one row that answers 「這個 Skill 在我的環境跑不跑得動」 was the only
    // row that never said 有差異. The other fourteen rows all reach for `.value`;
    // this one was the omission, not the convention.
    signature: (skill) =>
      `${skill.compatibility.spec_validation.value}/${skill.compatibility.capability.value}/${skill.compatibility.runtime.value}`,
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
          <p className="note">
            建立時間：
            <Timestamp at={skill.version.created_at} />
          </p>
        </>
      ),
  },
];

/** The table itself, taking already-loaded skills. */
export function CompareTable({ skills }: { skills: SkillDetail[] }) {
  return (
    <>
      {/*
        §3 item 9: the most important block on this page had only a `<caption>`,
        so the outline was 「h1 並排比較」 and nothing else — the answer was on
        screen with no heading marking it as the answer. `RunCompare` already
        fixed the same defect with an `<h2>` above its matrix; this is that.
        The caption stays: it says what the table's data IS (static, from
        import), which is a different sentence from what the section is.
      */}
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
              // An absent value is its own value here: one skill declaring MIT and another
              // declaring nothing is a difference, and the sentinel keeps it from
              // colliding with a genuine empty-ish signature.
              const differs = new Set(signatures.map((value) => value ?? "\u0000未提供")).size > 1;
              return (
                <tr key={row.label} className={differs ? "compare-differs" : undefined}>
                  <th scope="row">
                    {row.label}
                    {differs && <span className="badge badge-differs">有差異</span>}
                  </th>
                  {skills.map((skill, index) => (
                    <td key={skill.skill_id}>
                      {signatures[index] === undefined ? (
                        <span className="compare-unknown">{row.absent?.(skill) ?? "未提供"}</span>
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
  // One representative error for the shared component. All three reads are the
  // same endpoint with the same session, so a 401 on one is a 401 on all.
  const firstError = results.find((result) => result.error)?.error;

  return (
    <section>
      <h1>並排比較</h1>
      <p className="note">
        以下全部來自靜態資料（匯入時記錄與掃描結果），沒有任何一項是試跑出來的。
      </p>

      {/* 「搜尋結果」以前是唯一的來源，而 2026-09-03 起目錄也有同一組勾選框
          （Home 的 `CompareBar` 在兩種狀態下都渲染），這句話沒有跟上。設計 §3
          第 14 條：同一件事在兩處講得不一樣。 */}
      {skillIds.length < 2 && (
        <p role="alert">請從首頁的搜尋結果或目錄選擇 2 到 3 個 Skill 再進行比較。</p>
      )}
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
      {/*
        The count stays — it is the fact this page owes the reader, and no
        single-error component can say 「2 of 3」. What is added under it is the
        first failure's own answer through the shared component: a page that
        said only 「讀取失敗」 to a logged-out visitor told them to retry a read
        that will refuse every time (資訊架構 §5 IA-6).
      */}
      {failed > 0 && (
        <>
          <p role="alert">有 {failed} 個 Skill 讀取失敗，未列入下表。</p>
          <ReadFailure error={firstError} what="這些 Skill" />
        </>
      )}

      {skills.length >= 2 && <CompareTable skills={skills} />}

      <p>
        {/* 回到首頁，而不是「回到搜尋」：這一頁到得了的來源有兩個（搜尋結果與目錄），
            而這條連結兩者都不帶參數，所以它送人回去的一律是不帶查詢的首頁——也就是
            目錄。名字要說出它真的會做什麼。查詢字串的往返（比完三筆想換掉一筆）需要
            /compare 的 validateSearch 收下 `q`、CompareBar 帶上它，那是一批獨立的
            改動，記在 04。 */}
        <Link to="/">回到首頁的目錄</Link>
      </p>
    </section>
  );
}
