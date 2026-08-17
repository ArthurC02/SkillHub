import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useForkSkill, useSkillDetail } from "../api/skills";
import { useMe } from "../api/me";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
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

  if (isLoading) return <p>載入中…</p>;
  // 410 is a different fact from 404: this skill existed and was withdrawn.
  if (error instanceof ApiError && error.status === 410) {
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  }
  if (error || !skill) return <p role="alert">找不到這個 Skill，或載入失敗。</p>;

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
        <p className="note">{skill.tier.note}</p>
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

      <Enrichment enrichment={skill.enrichment} />

      <Limitations limitations={skill.limitations} />

      <section>
        <h2>來源</h2>
        {skill.source ? <SourceBlock source={skill.source} /> : <p>沒有保存任何來源紀錄。</p>}
      </section>

      <section>
        <h2>License</h2>
        <LicenseBadge license={skill.license} />
        <LicenseNotes license={skill.license} />
      </section>

      <Redistribution skill={skill} />

      <section>
        <h2>風險揭露</h2>
        <RiskIndicator risk={skill.risk} />
      </section>

      <section>
        <h2>相容性</h2>
        <CompatibilityStatus compatibility={skill.compatibility} />
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
            <li>建立時間：{skill.version.created_at}</li>
          </ul>
        ) : (
          <p>這個 Skill 還沒有已保存的版本內容。</p>
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

      <ForkAction skillId={skillId} isLoggedIn={!!me} />
    </article>
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
 * The section is omitted entirely when the API sends no `redistribution`: the
 * contract requires it and the current detail handler does not send it yet, and
 * an absent field is not a third verdict to render. The link stays offered in
 * that case because the packaging endpoints fail closed on their own — the page
 * they lead to states the refusal.
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
          <p className="note">{skill.redistribution.note}</p>
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
        <p className="note">這個 Skill 還沒有已保存的版本內容，沒有東西可以打包。</p>
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
 */
function Limitations({ limitations }: { limitations: SkillLimitation[] }) {
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
      <h2>
        白話摘要
        <span className="badge badge-source-model" title="由模型產生，未經人工核對">
          AI 產生
        </span>
      </h2>
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
      {source.fetched_at && <p>擷取時間：{source.fetched_at}</p>}

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
        <p className="note">最近一次來源可用性檢查：{source.last_checked_at}（當時可取得）。</p>
      ) : (
        <p className="note">尚未檢查過來源是否仍可取得。</p>
      )}

      <p className="note">{source.trust.note}</p>
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
    return <p className="login-prompt">登入後即可 Fork 這個 Skill 到你的工作區。</p>;
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
