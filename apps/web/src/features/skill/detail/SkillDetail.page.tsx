import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import { useSkillDetail } from "../skills.service";
import { useMe } from "../../../core/session/me.service";
import { CompatibilityStatus } from "../../../shared/ui/CompatibilityStatus";
import { GeneratedNotice } from "../../creation";
import { LabelledBadge } from "../../../shared/ui/LabelledBadge";
import { RiskIndicator } from "../../../shared/ui/RiskIndicator";
import { VersionUpload } from "./components/VersionUpload";
import { VersionHistory } from "./components/VersionHistory";
import { CategoryEditor } from "./components/CategoryEditor";
import { Redistribution } from "./components/Redistribution";
import { Limitations } from "./components/Limitations";
import { Enrichment } from "./components/Enrichment";
import { SourceBlock } from "./components/SourceBlock";
import { TrialEntry } from "./components/TrialEntry";
import { ForkAction } from "./components/ForkAction";
import "./SkillDetail.page.css";

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
