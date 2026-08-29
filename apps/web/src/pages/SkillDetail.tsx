import { useState } from "react";
import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Timestamp } from "../components/Timestamp";
import { VersionDiff } from "./RunCompare";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useForkSkill, useSkillDetail, useSkillVersions, skillDiffUrl } from "../api/skills";
import { useMe } from "../api/me";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import { PACKAGING_BLOCKED_LABEL, packagingGate } from "./Packaging";
import type {
  SkillDetail as SkillDetailModel,
  SkillEnrichment,
  SkillLimitation,
  SkillSource,
  SkillTags,
} from "../api/types";

/**
 * DISC-006/008: general-mode skill detail, reading GET /api/skills/{id}.
 * Anonymous callers get the public catalog (DISC-010).
 *
 * Progressive disclosure: the plain-language answer is on the page, and the
 * identifiers that only matter when you are checking someone's work (hashes,
 * version ids, which model wrote the summary) sit behind <details>.
 */
export function SkillDetail() {
  const { skillId } = useParams({ from: "/skills/$skillId" });
  const { data: skill, isLoading, error } = useSkillDetail(skillId);
  const { data: me } = useMe();

  if (isLoading) return <Loading what="這個 Skill" />;
  // 410 is a different fact from 404: this skill existed and was withdrawn.
  if (error instanceof ApiError && error.status === 410) {
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  }
  // Everything else through the shared component (資訊架構 §5 IA-6): a 401 says
  // to log in, and every other status keeps the server's own message instead of
  // being flattened into 「找不到這個 Skill，或載入失敗。」 — which answered a 500
  // with 「找不到」, i.e. with the wrong fact.
  if (error) return <ReadFailure error={error} what="這個 Skill" />;
  if (!skill) return <p role="alert">找不到這個 Skill。</p>;

  return (
    <article>
      <header>
        <h1>{skill.name}</h1>
        {/* The package author's own frontmatter description, never the model's. */}
        <p>{skill.summary}</p>
        <div className="badge-row">
          <LabelledBadge kind="tier" value={skill.tier} />
          {skill.source && <LabelledBadge kind="trust" value={skill.source.trust} />}
          <LicenseBadge license={skill.license} />
        </div>
      </header>

      {/*
        0023 licensing hold. Above everything else on the page, because it
        changes what the rest of the page can be used for: no full text, no file
        tree, no run. role="status" rather than "alert" — it is a standing
        condition of this listing, not something that just went wrong.
      */}
      {skill.access_restriction && (
        <section className="notice" role="status">
          <h2>授權審查中,部分功能已關閉</h2>
          <p>{skill.access_restriction.note}</p>
        </section>
      )}

      {/*
        The four human-checkable verdicts come first (system.md §1.1 + §3 item
        1). They used to sit below ~400px of model-written 白話摘要: 可散布性 at
        roughly y1174, 風險揭露 at y1297, 相容性 at y1507 in a 900px viewport, so
        on the page where 「這個別人寫的東西可不可信」 is asked, the first screen was
        half model output and the scan result was 1.4 viewports down.

        The AI summary losing its top slot is the intent, not a side effect
        (§2.6): a model's restatement of the package is not the answer to that
        question, and it is the one block here that nobody has checked.
      */}
      <section>
        <h2>風險揭露</h2>
        <RiskIndicator risk={skill.risk} />
      </section>

      <Redistribution skill={skill} />

      <section>
        <h2>License</h2>
        <LicenseBadge license={skill.license} />
        <LicenseNotes license={skill.license} />
      </section>

      <section>
        <h2>相容性</h2>
        <CompatibilityStatus compatibility={skill.compatibility} />
      </section>

      <Enrichment enrichment={skill.enrichment} />

      <Limitations limitations={skill.limitations} />

      {/*
        GEN-004 wants this on the detail page and on the workspace list, and the
        two have to answer it the same way. This used to read the VERSION's
        source (`skill.source.type`), which is `upload` for any version the user
        saved themselves — so the moment a generated skill got a second version,
        the detail page silently dropped the two absences while the list, which
        reads the skill row, went on showing them. The skill row is the right
        source: `redistribution` is what GEN-007's search exclusion keys on, so
        it is exactly the set of skills the disclosure is about. Same expression
        as WorkspaceSkills.tsx, one level deeper because detail sends a Labelled.
      */}
      {skill.redistribution?.value === "generated" && <GeneratedNotice skillId={skill.skill_id} />}

      <section>
        <h2>來源</h2>
        {skill.source ? <SourceBlock source={skill.source} /> : <p>沒有保存任何來源紀錄。</p>}
      </section>

      {skill.allowed_tools && skill.allowed_tools.length > 0 && (
        <section>
          <h2>套件宣告可用的工具</h2>
          <ul>
            {skill.allowed_tools.map((tool) => (
              <li key={tool}>
                <code>{tool}</code>
              </li>
            ))}
          </ul>
          <p className="note">以上為套件自行宣告的 allowed-tools，未經驗證。</p>
        </section>
      )}

      <section>
        <h2>{skill.derivation.label}</h2>
        <p className="note">{skill.derivation.note}</p>
        {skill.derivation.is_fork && skill.derivation.forked_from_skill_id && (
          <p>
            <Link to="/skills/$skillId" params={{ skillId: skill.derivation.forked_from_skill_id }}>
              查看原始 Skill
            </Link>
          </p>
        )}
      </section>

      <VersionHistory skillId={skillId} />

      <details>
        <summary>進階資訊（版本與識別碼）</summary>
        {skill.version ? (
          <ul>
            <li>版本編號：v{skill.version.version_number}</li>
            <li>
              版本 ID：<code>{skill.version.version_id}</code>
            </li>
            <li>
              內容雜湊：<code>{skill.version.content_hash}</code>
            </li>
            <li>
              建立時間：
              <Timestamp at={skill.version.created_at} />
            </li>
          </ul>
        ) : (
          /* 設計 §2.9: 別人的 Skill 的版本清單回空陣列，而「沒有版本」與
             「你看不到」是兩件事——前者會被讀成這個 Skill 是空的。表列詞是
             「無權檢視」（ADR-011 的 Workspace scope）。 */
          <p>
            無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
            之後才會有屬於你的版本；這不代表它沒有版本。
          </p>
        )}
        {skill.derivation.forked_from_version_id && (
          <p>
            分岔自版本：<code>{skill.derivation.forked_from_version_id}</code>
          </p>
        )}
      </details>

      {/*
        0023: with a licensing hold open, the advanced view is closed and the
        link goes with it — a link that leads to a 403 is worse than no link.
        The reason is stated where the link used to be, so the absence reads as
        a decision rather than as a missing feature.
      */}
      {skill.version && !skill.access_restriction && (
        <nav>
          <Link to="/skills/$skillId/files" params={{ skillId }}>
            查看 SKILL.md 與檔案樹（進階模式）
          </Link>
        </nav>
      )}

      {/*
        「這個 Skill 我寫過哪些 Test Case」 had no route until the list grew a
        `skill_id` filter. Signed-in only, for the same reason the fork action
        is: a Test Case belongs to a workspace, and a link to an empty list is
        what an anonymous reader would get.
      */}
      {me && (
        <section>
          <h2>試跑</h2>
          <p>
            <Link to="/lab/test-cases" search={{ skill: skillId }}>
              此 Skill 的 Test Case
            </Link>
          </p>
          <p className="note">Test Case 是試跑用的草稿：User Prompt、測試資料與驗收條件。</p>
        </section>
      )}

      <ForkAction skillId={skillId} isLoggedIn={!!me} />
    </article>
  );
}

/**
 * 02:WS-001 第 4 條「使用者可查看任兩版本的內容差異」, and the version list it needs
 * to stand on.
 *
 * Neither existed anywhere in the app. `useSkillVersions` had one call site —
 * `RunPreflight`'s 「要跑哪一版」 picker — and the detail page printed only the
 * CURRENT version's four fields, so the product's own loop ended blind: 採用改善
 * 建議 creates a new immutable version (iron rule 4, `createVersionFromSuggestions`)
 * and nothing could show what it changed. `WorkspaceSkills.tsx`'s header claimed
 * the history was 「reachable only through its detail page and the diff route」 —
 * a path nobody had laid.
 *
 * **NO NEW ROUTE**, on purpose, and it is 資訊架構 §0.1 that says so rather than
 * frugality: R2 puts a single item at `/skills/$id`, and a version of a skill is
 * that skill at a moment, not a second object with its own address. R3 would
 * then want two ways into whatever address was invented, and there is one place
 * that produces the context (this page). R1 is satisfied too — the page answers
 * 「這個 Skill 可不可信」 and a version's diff is evidence for it, the same way
 * 風險揭露 above is.
 *
 * A CHILD COMPONENT, not inlined: the page returns early on loading and on 410,
 * so a hook added to `SkillDetail` itself would change hook order between
 * renders.
 *
 * `useSkillVersions` is session-scoped, and the list of somebody else's skill is
 * empty rather than forbidden — so the two absences are worded apart the way
 * §2.9 requires, and `ReadFailure` carries 401 to 「需要登入」 without swallowing
 * any other status.
 */
function VersionHistory({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  // The pair being compared, or nothing. Not in the URL: 資訊架構 §0.1 R4 —
  // 「你在看哪一份東西」 is the skill, and which two of its versions are expanded
  // is 「你偏好怎麼看」, the same call IA-4 made for the Trace reading mode.
  const [pair, setPair] = useState<{ from: string; to: string } | null>(null);

  const list = versions.data?.versions ?? [];

  return (
    <section>
      <h2>版本歷史</h2>
      {versions.isPending && <Loading what="版本歷史" />}
      <ReadFailure error={versions.error} what="版本歷史" />

      {versions.data &&
        (list.length === 0 ? (
          /* §2.9 again: an empty list here is 無權檢視, not 「這個 Skill 沒有
             版本」. Every skill has at least one — the one that was imported. */
          <p>
            無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
            之後才會有屬於你的版本；這不代表它沒有版本。
          </p>
        ) : (
          <>
            <ul className="search-results">
              {list.map((version, index) => {
                // Newest first (the endpoint's order), so 「上一版」 is the NEXT
                // element. The oldest row has none, and says why rather than
                // rendering a control that would compare a version with itself
                // (§2.4: a missing control owes a reason too).
                const previous = list[index + 1];
                const open = pair?.to === version.version_id;
                return (
                  <li key={version.version_id} className="search-result">
                    <p>
                      <strong>v{version.version_number}</strong>
                      <span className="note">
                        建立時間：
                        <Timestamp at={version.created_at} />
                      </span>
                    </p>
                    {previous ? (
                      <p>
                        <button
                          type="button"
                          onClick={() =>
                            setPair(
                              open ? null : { from: previous.version_id, to: version.version_id },
                            )
                          }
                        >
                          {open ? "收起與上一版的比較" : "與上一版比較"}
                        </button>
                      </p>
                    ) : (
                      <p className="note">這是最早的版本，沒有上一版可以比較。</p>
                    )}
                    {open && <VersionDiff url={skillDiffUrl(skillId, pair.from, pair.to)} />}
                  </li>
                );
              })}
            </ul>
            <p className="note">
              版本不可變（ADR-003）：採用改善建議會建立新的一版，舊的一版原封不動留著。
              差異比對的是兩版套件檔案的內容，不是它們的試跑結果。
            </p>
          </>
        ))}
    </section>
  );
}

/**
 * 02:SEC-007 / ADR-027 決策 4 — the three-state redistribution answer, and the
 * packaging entry point that depends on it.
 *
 * The entry is closed for `blocked` and for `unknown` alike, in the words
 * `PACKAGING_BLOCKED_LABEL` uses on the download page itself: one table, two
 * surfaces, so the reason a user is refused here and the reason they would be
 * refused there cannot drift apart. A licensing hold closes it too and is
 * reported as the hold rather than as a licensing verdict — they are two
 * independent locks and only one of them is temporary.
 *
 * The contract requires `redistribution` on every skill, so the badge is the
 * normal case. A response without it is a platform that failed to answer, not a
 * fourth state: the section says that plainly instead of rendering a verdict
 * nobody gave, and `packagingGate` closes the entry all the same.
 */
function Redistribution({ skill }: { skill: SkillDetailModel }) {
  const blocked = packagingGate(skill);

  return (
    <section>
      <h2>可散布性與打包</h2>
      {skill.redistribution ? (
        <>
          <p>
            <LabelledBadge kind="redistribution" value={skill.redistribution} />
          </p>
        </>
      ) : (
        <p className="note">平台沒有回報這個 Skill 的可散布性判定。</p>
      )}

      {blocked ? (
        <>
          <p>
            <button type="button" disabled>
              打包並下載
            </button>
          </p>
          <p className="note">{PACKAGING_BLOCKED_LABEL[blocked]}</p>
        </>
      ) : skill.version ? (
        <p>
          <Link
            to="/skills/$skillId/package"
            params={{ skillId: skill.skill_id }}
            search={{ version: skill.version.version_id }}
          >
            打包並下載這個版本
          </Link>
        </p>
      ) : (
        /* 同上，§2.9 的「無權檢視」。 */
        <p className="note">
          無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
          之後才會有屬於你的版本；這不代表它沒有版本。沒有版本內容就沒有東西可以打包。
        </p>
      )}
    </section>
  );
}

/**
 * 限制 (02:DISC-003 一般模式). The API assembles this list from two sources and
 * labels each entry with the one it came from: `model` is the enrichment
 * restating what the document says about its own limits, `scan` is derived from
 * the static package scan. ADR-013 requires the model half to be visibly marked,
 * so the two are never merged into one anonymous sentence.
 *
 * An empty list says so explicitly. Dropping the section would read as "this
 * skill has no limits", which is the opposite of what an empty list means here:
 * neither source stated one.
 *
 * What each marker means is visible text below the list, not only the badge's
 * `title` (system.md §3 item 4). These are the two sentences that stop a reader
 * over-trusting a badge, and on a touch device a tooltip is not there at all.
 * Stated per marker actually present, so the page never explains a label it did
 * not render.
 */
function Limitations({ limitations }: { limitations: SkillLimitation[] }) {
  const fromModel = limitations.some((l) => l.source === "model");
  const fromScan = limitations.some((l) => l.source !== "model");

  return (
    <section>
      <h2>限制</h2>
      {limitations.length === 0 ? (
        <p className="note">
          沒有任何來源指出限制——這代表沒有人說明過，不代表這個 Skill 沒有限制。
        </p>
      ) : (
        <ul className="risk-list">
          {limitations.map((limitation) => (
            <li key={`${limitation.source}-${limitation.text}`}>
              {limitation.text}
              {limitation.source === "model" ? (
                <span className="badge badge-source-model" title="由模型重述套件內容，未經人工核對">
                  AI 產生
                </span>
              ) : (
                <span className="badge badge-source-template" title="由匯入時的靜態掃描結果推得">
                  掃描推得
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {fromModel && <p className="note">「AI 產生」的項目由模型重述套件內容，未經人工核對。</p>}
      {fromScan && (
        <p className="note">「掃描推得」的項目由匯入時的靜態掃描結果推得，掃描不執行套件內容。</p>
      )}
    </section>
  );
}

const TAG_BUCKETS: Array<{ key: keyof SkillTags; label: string }> = [
  { key: "inputs", label: "輸入" },
  { key: "outputs", label: "輸出" },
  { key: "tools", label: "會用到的工具" },
  { key: "dependencies", label: "依賴" },
];

/**
 * ADR-013: index-time model output, always labelled as model-written so a
 * reader can tell it from the author's own text above.
 */
function Enrichment({ enrichment }: { enrichment: SkillEnrichment }) {
  if (enrichment.status !== "enriched") {
    return (
      <section>
        <h2>白話摘要</h2>
        <p className="note">{enrichment.note}</p>
        {/*
          The 輸入／輸出／依賴 rows stay on the page while enrichment is pending,
          reading 未知. They come from the enrichment, so a pending row has none
          — and omitting the rows entirely would present a skill with unknown
          inputs as a skill that takes none.
        */}
        {TAG_BUCKETS.map(({ key, label }) => (
          <p key={key}>
            {label}：<span className="note">未知（尚未產生索引摘要）</span>
          </p>
        ))}
      </section>
    );
  }

  return (
    <section>
      {/*
        The marker is beside the heading, not inside it. Inside, it joined the
        accessible name — `__outlines__/skills-skillId.txt` recorded the result
        as `h2 白話摘要AI 產生`, which is both the glued rendering §3 item 15
        forbids and a heading that names two things. A `.badge-row` is the
        surface this app already uses for a badge that qualifies the block
        below it (the header above does the same with three of them).
      */}
      <h2>白話摘要</h2>
      <p className="badge-row">
        <span className="badge badge-source-model" title="由模型產生，未經人工核對">
          AI 產生
        </span>
        <span className="note">由模型產生，未經人工核對</span>
      </p>
      {enrichment.summary && <p>{enrichment.summary}</p>}

      {enrichment.task_examples && enrichment.task_examples.length > 0 && (
        <>
          <h3>可以用來做什麼（AI 產生的任務範例）</h3>
          <ul>
            {enrichment.task_examples.map((example) => (
              <li key={example}>{example}</li>
            ))}
          </ul>
        </>
      )}

      {/*
        DISC-003 asks for 輸入、輸出、依賴 as separate facts, and the contract
        keeps them in separate buckets, so they are not flattened back into one
        anonymous tag cloud here.

        An empty bucket is stated as 未知 rather than dropped. A missing row
        reads as "this skill needs no dependencies"; what it actually means is
        that nothing extracted any, which is the 不得自行推定 case 02:DISC-004
        names. The same reasoning as the search row's 依賴 column.
      */}
      {enrichment.tags &&
        TAG_BUCKETS.map(({ key, label }) =>
          enrichment.tags![key].length > 0 ? (
            <p key={key}>
              {label}：
              <span className="tag-list">
                {enrichment.tags![key].map((tag) => (
                  <span key={tag} className="badge">
                    {tag}
                  </span>
                ))}
              </span>
            </p>
          ) : (
            <p key={key}>
              {label}：<span className="note">未知（沒有擷取到，不代表沒有）</span>
            </p>
          ),
        )}

      <p className="note">{enrichment.note}</p>
      {(enrichment.model || enrichment.prompt_version) && (
        <details>
          <summary>產生這段摘要的模型</summary>
          <ul>
            {enrichment.model && (
              <li>
                模型：<code>{enrichment.model}</code>
              </li>
            )}
            {enrichment.prompt_version && (
              <li>
                Prompt 版本：<code>{enrichment.prompt_version}</code>
              </li>
            )}
          </ul>
        </details>
      )}
    </section>
  );
}

/** DISC-003: URL, version/commit, fetch time and content hash of what arrived. */
function SourceBlock({ source }: { source: SkillSource }) {
  if (source.type === "generated") {
    return <GeneratedSourceBlock source={source} />;
  }
  return (
    <>
      <p>匯入方式：{source.type === "git" ? "從 Git 來源擷取" : "使用者上傳"}</p>
      {source.url && (
        <p>
          來源網址：{" "}
          <a href={source.url} rel="noreferrer noopener">
            {source.url}
          </a>
        </p>
      )}
      {source.source_version && (
        <p>
          來源版本／Commit：<code>{source.source_version}</code>
        </p>
      )}
      {source.fetched_at && (
        <p>
          擷取時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}

      {/*
        The probe's two facts stay apart: "gone since when" is the one that
        matters, and "never probed" is rendered as unknown rather than quietly
        omitted, because an absent line reads as reassurance.
      */}
      {source.unavailable_since ? (
        <p className="badge badge-risk">
          來源已失效，自 {source.unavailable_since} 起無法取得。目前顯示的是失效前保存的內容。
        </p>
      ) : source.last_checked_at ? (
        <p className="note">
          最近一次來源可用性檢查：
          <Timestamp at={source.last_checked_at} />
          （當時可取得）。
        </p>
      ) : (
        <p className="note">尚未檢查過來源是否仍可取得。</p>
      )}

      {source.content_hash && (
        <details>
          <summary>內容雜湊</summary>
          <code>{source.content_hash}</code>
        </details>
      )}
    </>
  );
}

/**
 * GEN-002's provenance block: a package with no upstream, whose entire source
 * record is the words the owner typed.
 *
 * A separate branch rather than three more optional lines in SourceBlock,
 * because almost every line there is about an upstream that does not exist
 * here — the availability probe, the commit, the URL. Rendering those as
 * unknown would describe this package as one whose origin nobody recorded, and
 * 02:GEN-002 forbids exactly that: **不得顯示為未知來源**. It is known. It is
 * not a URL.
 */
function GeneratedSourceBlock({ source }: { source: SkillSource }) {
  return (
    <>
      <p>來源：由平台依你的任務描述生成</p>
      {source.task_description && (
        <details>
          <summary>你當時輸入的任務描述</summary>
          <p>{source.task_description}</p>
        </details>
      )}
      {source.fetched_at && (
        <p>
          生成時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}
      {source.generator_model && (
        <p>
          模型：<code>{source.generator_model}</code>
        </p>
      )}
      {source.generator_prompt_version && (
        <p>
          提示詞版本：<code>{source.generator_prompt_version}</code>
        </p>
      )}
      {source.content_hash && (
        <details>
          <summary>內容雜湊</summary>
          <code>{source.content_hash}</code>
        </details>
      )}
    </>
  );
}

function ForkAction({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const fork = useForkSkill();

  if (!isLoggedIn) {
    // `login-prompt` carried no CSS rule and no test selected it, so it was a
    // class that only looked like a hook. Removed rather than left to be read
    // as one (same call as `badge-risk-none` in RiskIndicator).
    return <p>登入後即可 Fork 這個 Skill 到你的工作區。</p>;
  }

  return (
    <div>
      <button type="button" onClick={() => fork.mutate(skillId)} disabled={fork.isPending}>
        {fork.isPending ? "Fork 中…" : "Fork 這個 Skill"}
      </button>
      {fork.isError && <p role="alert">Fork 失敗，請稍後再試。</p>}
      {fork.isSuccess && (
        <p>
          已建立 Fork：
          <Link to="/skills/$skillId" params={{ skillId: fork.data.skill_id }}>
            {fork.data.name}
          </Link>
        </p>
      )}
    </div>
  );
}
