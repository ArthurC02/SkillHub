import { useState } from "react";
import { Loading } from "../components/Loading";
import { ReadFailure } from "../components/LoginRequired";
import { Timestamp } from "../components/Timestamp";
import { VersionDiff } from "./RunCompare";
import { Link, useParams } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiFetch } from "../api/client";
import { useForkSkill, useSkillDetail, useSkillVersions, skillDiffUrl } from "../api/skills";
import { useMe } from "../api/me";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { GeneratedNotice } from "../components/GeneratedNotice";
import { LabelledBadge } from "../components/LabelledBadge";
import { LicenseBadge, LicenseNotes } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import { SignInAction } from "../components/SignIn";
import { VersionUpload } from "../components/VersionUpload";
import { PACKAGING_BLOCKED_LABEL, packagingGate } from "./Packaging";
import type { SetSkillCategoryRequest } from "@skillhub/api-client-ts";
import type {
  Labelled,
  SkillDetail as SkillDetailModel,
  SkillEnrichment,
  SkillLimitation,
  SkillSource,
  SkillTags,
} from "../api/types";

export function SkillDetail() {
  const { skillId } = useParams({ from: "/skills/$skillId" });
  const { data: skill, isLoading, error } = useSkillDetail(skillId);
  const { data: me } = useMe();

  if (isLoading) return <Loading what="這個 Skill" />;
  if (error instanceof ApiError && error.status === 410) {
    return <p role="alert">這個 Skill 已從目錄下架，內容不再提供。</p>;
  }
  if (error) return <ReadFailure error={error} what="這個 Skill" />;
  if (!skill) return <p role="alert">找不到這個 Skill。</p>;

  return (
    <article>
      <div className="detail-layout">
        <div className="detail-main">
          <header className="detail-answer">
            <h1>{skill.name}</h1>
            <p>{skill.summary}</p>
            <div className="badge-row">
              <LabelledBadge kind="category" value={skill.category} />
              <LabelledBadge kind="tier" value={skill.tier} />
              {skill.source && <LabelledBadge kind="trust" value={skill.source.trust} />}
            </div>
          </header>

          {skill.access_restriction && (
            <section className="notice notice-danger" role="status">
              <h2>授權審查中,部分功能已關閉</h2>
              <p>{skill.access_restriction.note}</p>
            </section>
          )}

          <div className="verdict-grid">
            <section>
              <h2>風險揭露</h2>
              <RiskIndicator risk={skill.risk} />
            </section>

            <Redistribution skill={skill} isLoggedIn={!!me} />

            <section>
              <h2>相容性</h2>
              <CompatibilityStatus compatibility={skill.compatibility} />
            </section>

            <section>
              <h2>套件宣告可用的工具</h2>
              {skill.allowed_tools && skill.allowed_tools.length > 0 ? (
                <>
                  <ul>
                    {skill.allowed_tools.map((tool) => (
                      <li key={tool}>
                        <code>{tool}</code>
                      </li>
                    ))}
                  </ul>
                  <p className="note">以上為套件自行宣告的 allowed-tools，未經驗證。</p>
                </>
              ) : skill.risk.scan_status === "unavailable" ? (
                <p className="note">
                  未測量——這個版本沒有靜態掃描結果可讀，所以平台不知道套件宣告了哪些工具。
                </p>
              ) : (
                <p className="note">
                  不適用——套件沒有宣告 allowed-tools。在 Agent Skills 的格式裡那代表
                  <strong>不設限</strong>，不代表它不用工具。
                </p>
              )}
            </section>
          </div>

          <section>
            <h2>它能做什麼</h2>
            <Enrichment enrichment={skill.enrichment} />
            <Limitations limitations={skill.limitations} />
          </section>

          {skill.redistribution?.value === "generated" && (
            <GeneratedNotice skillId={skill.skill_id} />
          )}

          <section>
            <h2>它從哪裡來</h2>
            {skill.source ? <SourceBlock source={skill.source} /> : <p>沒有保存任何來源紀錄。</p>}

            <h3>{skill.derivation.label}</h3>
            <p className="note">{skill.derivation.note}</p>
            {skill.derivation.is_fork && skill.derivation.forked_from_skill_id && (
              <p>
                <Link
                  to="/skills/$skillId"
                  params={{ skillId: skill.derivation.forked_from_skill_id }}
                >
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
              <p>無權檢視——這個工作區看不到這個 Skill 的版本內容（原因見上面的〈版本〉）。</p>
            )}
            {skill.derivation.forked_from_version_id && (
              <p>
                分岔自版本：<code>{skill.derivation.forked_from_version_id}</code>
              </p>
            )}
          </details>
        </div>

        <aside className="detail-rail" aria-label="這個 Skill 的操作">
          <TrialEntry skillId={skillId} isLoggedIn={!!me} />

          <section>
            <h3>Fork 到你的工作區</h3>
            <ForkAction skillId={skillId} isLoggedIn={!!me} />
          </section>

          {skill.version && !skill.access_restriction && (
            <nav>
              <Link to="/skills/$skillId/files" params={{ skillId }}>
                查看 SKILL.md 與檔案樹（進階模式）
              </Link>
            </nav>
          )}

          <VersionUpload skillId={skillId} />

          <CategoryEditor skillId={skillId} category={skill.category} />
        </aside>
      </div>
    </article>
  );
}

function VersionHistory({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  const [pair, setPair] = useState<{ from: string; to: string } | null>(null);

  const list = versions.data?.versions ?? [];

  return (
    <section>
      <h2>版本</h2>
      {versions.isPending && <Loading what="版本歷史" />}
      <ReadFailure error={versions.error} what="版本歷史" />

      {versions.data &&
        (list.length === 0 ? (
          <p>
            無權檢視——這個工作區看不到這個 Skill 的版本內容。別人的 Skill 要 Fork
            之後才會有屬於你的版本；這不代表它沒有版本。
          </p>
        ) : (
          <>
            <p>
              共 {list.length} 版，最新 v{list[0].version_number}（
              <Timestamp at={list[0].created_at} />）
            </p>
            <details>
              <summary>每一版與它跟上一版的差異</summary>
              <ul className="search-results">
                {list.map((version, index) => {
                  const previous = list[index + 1];
                  const open = pair?.to === version.version_id;
                  return (
                    <li key={version.version_id} className="search-result">
                      <p>
                        <strong>v{version.version_number}</strong>{" "}
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
                版本不可變：採用改善建議會建立新的一版。差異比對的是套件內容，不是試跑結果。
              </p>
            </details>
          </>
        ))}
    </section>
  );
}

const CATEGORY_CHOICES: { value: SetSkillCategoryRequest["category"]; label: string }[] = [
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
  { value: "unassigned", label: "尚未定值" },
];

function CategoryEditor({ skillId, category }: { skillId: string; category: Labelled }) {
  const versions = useSkillVersions(skillId);
  const client = useQueryClient();
  const [choice, setChoice] = useState<SetSkillCategoryRequest["category"]>(
    category.value as SetSkillCategoryRequest["category"],
  );

  const save = useMutation({
    mutationFn: () =>
      apiFetch(`/skills/${skillId}/category`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ category: choice }),
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["skills", skillId] }),
  });

  if ((versions.data?.versions.length ?? 0) === 0) return null;

  return (
    <section>
      <h3>類別</h3>
      <p className="field">
        <label htmlFor="skill-category">這個 Skill 是做什麼用的</label>
        <select
          id="skill-category"
          value={choice}
          onChange={(e) => setChoice(e.target.value as SetSkillCategoryRequest["category"])}
        >
          {CATEGORY_CHOICES.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>
      </p>
      <button type="button" onClick={() => save.mutate()} disabled={save.isPending}>
        {save.isPending ? "儲存中…" : "儲存"}
      </button>
      {save.isError && (
        <ReadFailure error={save.error} what="設定類別">
          <p role="alert">類別沒有設定成功，可以再按一次。</p>
        </ReadFailure>
      )}
      {save.isSuccess && <p role="status">類別已更新。</p>}
    </section>
  );
}

function Redistribution({ skill, isLoggedIn }: { skill: SkillDetailModel; isLoggedIn: boolean }) {
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
            <button type="button" disabled aria-describedby="packaging-blocked-reason">
              打包並下載
            </button>
          </p>
          <p className="note" id="packaging-blocked-reason">
            {PACKAGING_BLOCKED_LABEL[blocked]}
          </p>
        </>
      ) : skill.version ? (
        <PackagingEntry skill={skill} isLoggedIn={isLoggedIn} />
      ) : (
        <p className="note">
          無權檢視——這個工作區看不到這個 Skill
          的版本內容，所以沒有東西可以打包（原因見下面的〈版本〉）。
        </p>
      )}

      <h3>License</h3>
      <LicenseBadge license={skill.license} />
      <LicenseNotes license={skill.license} />
    </section>
  );
}

function PackagingEntry({ skill, isLoggedIn }: { skill: SkillDetailModel; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skill.skill_id);

  if (!isLoggedIn)
    return (
      <div className="note">
        打包與下載需要登入，而且只打包得了你自己工作區裡的版本——別人的 Skill 要先 Fork 一份。{" "}
        <SignInAction />
      </div>
    );
  if (versions.isPending) return <Loading what="這個 Skill 在你工作區的版本" />;
  if (versions.error) return <ReadFailure error={versions.error} what="這個 Skill 的版本" />;
  if ((versions.data?.versions.length ?? 0) === 0)
    return (
      <p className="note">
        這個 Skill 不在你的工作區，所以沒有屬於你的版本可以打包。
        <strong>要先 Fork 一份</strong>——旁邊的「Fork 到你的工作區」就是那一步。
      </p>
    );

  return (
    <p>
      <Link
        className="action"
        to="/skills/$skillId/package"
        params={{ skillId: skill.skill_id }}
        search={{ version: skill.version!.version_id }}
      >
        打包並下載這個版本
      </Link>
    </p>
  );
}

function Limitations({ limitations }: { limitations: SkillLimitation[] }) {
  const fromModel = limitations.some((l) => l.source === "model");
  const fromScan = limitations.some((l) => l.source !== "model");

  return (
    <>
      <h3>限制</h3>
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
                <span className="badge badge-source-model">AI 產生</span>
              ) : (
                <span className="badge badge-source-template">掃描推得</span>
              )}
            </li>
          ))}
        </ul>
      )}
      {fromModel && <p className="note">「AI 產生」的項目由模型重述套件內容，未經人工核對。</p>}
      {fromScan && (
        <p className="note">「掃描推得」的項目由匯入時的靜態掃描結果推得，掃描不執行套件內容。</p>
      )}
    </>
  );
}

const TAG_BUCKETS: Array<{ key: keyof SkillTags; label: string }> = [
  { key: "inputs", label: "輸入" },
  { key: "outputs", label: "輸出" },
  { key: "tools", label: "會用到的工具" },
  { key: "dependencies", label: "依賴" },
];

function Enrichment({ enrichment }: { enrichment: SkillEnrichment }) {
  if (enrichment.status !== "enriched") {
    return (
      <>
        <p className="note">{enrichment.note}</p>
        {TAG_BUCKETS.map(({ key, label }) => (
          <p key={key}>
            {label}：<span className="note">未知（尚未產生索引摘要）</span>
          </p>
        ))}
      </>
    );
  }

  return (
    <>
      <p className="badge-row">
        <span className="badge badge-source-model">AI 產生</span>
      </p>
      {enrichment.summary && <p>{enrichment.summary}</p>}

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
              {label}：<span className="note">未測量（沒有擷取到，不代表沒有）</span>
            </p>
          ),
        )}

      {enrichment.task_examples && enrichment.task_examples.length > 0 && (
        <details>
          <summary>可以用來做什麼（AI 產生的任務範例）</summary>
          <ul>
            {enrichment.task_examples.map((example) => (
              <li key={example}>{example}</li>
            ))}
          </ul>
        </details>
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
    </>
  );
}

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
      {source.fetched_at && (
        <p>
          擷取時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}

      {source.unavailable_since ? (
        <p className="badge badge-risk">
          來源已失效，自 <Timestamp at={source.unavailable_since} />{" "}
          起無法取得。目前顯示的是失效前保存的內容。
        </p>
      ) : !source.last_checked_at ? (
        <p className="note">尚未檢查過來源是否仍可取得。</p>
      ) : null}

      {(source.source_version ||
        source.content_hash ||
        (!source.unavailable_since && source.last_checked_at)) && (
        <details>
          <summary>識別碼</summary>
          <ul>
            {!source.unavailable_since && source.last_checked_at && (
              <li>
                最近一次來源可用性檢查：
                <Timestamp at={source.last_checked_at} />
                （當時可取得）
              </li>
            )}
            {source.source_version && (
              <li>
                來源版本／Commit：<code>{source.source_version}</code>
              </li>
            )}
            {source.content_hash && (
              <li>
                內容雜湊：<code>{source.content_hash}</code>
              </li>
            )}
          </ul>
        </details>
      )}
    </>
  );
}

function GeneratedSourceBlock({ source }: { source: SkillSource }) {
  const diagram = source.generation_inputs?.diagram;
  const references = source.generation_inputs?.references;
  return (
    <>
      <p>
        {source.task_description
          ? "來源：由平台依你的任務描述生成"
          : diagram
            ? "來源：由平台依你上傳的流程圖生成"
            : "來源：由平台生成"}
      </p>
      {source.task_description && (
        <details>
          <summary>你當時輸入的任務描述</summary>
          <p>{source.task_description}</p>
        </details>
      )}
      {source.generation_inputs && (
        <details>
          <summary>這一次生成用到的輸入</summary>
          {diagram && (
            <p>
              流程圖：{diagram.media_type}，{diagram.bytes} bytes，sha256{" "}
              <code>{diagram.sha256}</code>
              <span className="note">（平台沒有保留圖片本身，只留下這個雜湊）</span>
            </p>
          )}
          {references && references.length > 0 && (
            <>
              <p>參考的 Skill：</p>
              <ul>
                {references.map((r) => (
                  <li key={r.version_id}>
                    <Link to="/skills/$skillId" params={{ skillId: r.skill_id }}>
                      {r.name}
                    </Link>
                  </li>
                ))}
              </ul>
            </>
          )}
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

// Ownership signal is the (workspace-scoped) versions list, empty for a non-owner —
// not skill.version, which is present for every caller including the catalogue.
// Own component (not inlined) so the page's early returns can't shift hook order.
function TrialEntry({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const versions = useSkillVersions(skillId);

  if (!isLoggedIn)
    return (
      <section>
        <h3>試跑</h3>
        <div>
          試跑屬於你的工作區。先登入並 Fork 一份，才會有屬於你的版本可以跑。 <SignInAction />
        </div>
      </section>
    );

  if (versions.isPending)
    return (
      <section>
        <h3>試跑</h3>
        <Loading what="這個 Skill 在你工作區的版本" />
      </section>
    );
  if (versions.error)
    return (
      <section>
        <h3>試跑</h3>
        <ReadFailure error={versions.error} what="這個 Skill 的版本" />
      </section>
    );

  const inMyWorkspace = (versions.data?.versions.length ?? 0) > 0;

  return (
    <section>
      <h3>試跑</h3>
      {inMyWorkspace ? (
        <>
          <p>
            <Link to="/lab/test-cases" search={{ skill: skillId }}>
              此 Skill 的 Test Case
            </Link>
          </p>
          <p className="note" data-role="teaching">
            Test Case 是試跑用的草稿：User Prompt、測試資料與驗收條件。
          </p>
        </>
      ) : (
        <p>
          這個 Skill 不在你的工作區。Test Case 屬於工作區，所以 Test Case 清單裡看不到它、建立表單的
          Skill 選單也選不到它——
          <strong>要先 Fork 一份</strong>，才會有屬於你的版本可以試跑。下方的「Fork
          到你的工作區」就是那一步。
        </p>
      )}
    </section>
  );
}

function forkErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以 Fork 沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409) return "你的工作區已經有同名的 Skill。";
  }
  return "Fork 沒有成功，可以再按一次。";
}

function ForkAction({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const fork = useForkSkill();
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;

  if (!isLoggedIn) {
    return (
      <div>
        登入後即可 Fork 這個 Skill 到你的工作區。 <SignInAction />
      </div>
    );
  }

  return (
    <div>
      <p className="note">平台目前只讓有封測邀請的帳號 Fork。</p>
      <button
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={() => fork.mutate(skillId)}
        disabled={fork.isPending}
      >
        {fork.isPending ? "建立中…" : "以這個 Skill 為起點建立我自己的"}
      </button>
      {fork.isError && (
        <ReadFailure error={fork.error} what="Fork 這個 Skill">
          <p role="alert">{forkErrorMessage(fork.error)}</p>
        </ReadFailure>
      )}
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
