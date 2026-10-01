import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import { useSkillDetail } from "../skills.service";
import { useMe } from "../../../core/session/me.service";
import { CompatibilityStatus } from "../../../shared/ui/CompatibilityStatus";
import { GeneratedNotice } from "../../creation";
import { LabelledBadge } from "../../../shared/ui/LabelledBadge";
import { RiskIndicator } from "../../../shared/ui/RiskIndicator";
import { VersionHistory } from "./components/VersionHistory";
import { CategoryEditor } from "./components/CategoryEditor";
import { Redistribution } from "./components/Redistribution";
import { Limitations } from "./components/Limitations";
import { Enrichment } from "./components/Enrichment";
import { AllowedToolsSection } from "./components/AllowedToolsSection";
import { SkillProvenanceSection } from "./components/SkillProvenanceSection";
import { SkillIdentifiers } from "./components/SkillIdentifiers";
import { TrialEntry } from "./components/TrialEntry";
import { ForkAction } from "./components/ForkAction";
import { SkillWorkspaceNav } from "../components/SkillWorkspaceNav";
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
    <article className="skill-detail">
      <header className="detail-answer">
        <h1>{skill.name}</h1>
        <p>{skill.summary}</p>
        <div className="badge-row">
          <LabelledBadge kind="category" value={skill.category} />
          <LabelledBadge kind="tier" value={skill.tier} />
          {skill.source && <LabelledBadge kind="trust" value={skill.source.trust} />}
        </div>
        <SkillWorkspaceNav skillId={skillId} versionId={skill.version?.version_id} />
      </header>

      <div className="detail-layout">
        <aside className="detail-rail" aria-label="這個 Skill 的操作">
          <TrialEntry skillId={skillId} isLoggedIn={!!me} />

          <section>
            <h2>Fork 到你的工作區</h2>
            <ForkAction skillId={skillId} isLoggedIn={!!me} />
          </section>

          {skill.version && !skill.access_restriction && (
            <nav>
              <Link to="/skills/$skillId/files" params={{ skillId }}>
                查看 SKILL.md 與檔案樹（進階模式）
              </Link>
            </nav>
          )}

          <CategoryEditor skillId={skillId} category={skill.category} />
        </aside>

        <div className="detail-main">
          {skill.access_restriction && (
            <section className="notice notice-danger" role="status">
              <h2>授權審查中,部分功能已關閉</h2>
              <p>{skill.access_restriction.note}</p>
            </section>
          )}

          <div className="verdict-grid" data-role="evidence">
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
              <AllowedToolsSection
                allowedTools={skill.allowed_tools}
                scanStatus={skill.risk.scan_status}
              />
            </section>
          </div>

          <section>
            <h2>它能做什麼</h2>
            <Enrichment enrichment={skill.enrichment} />
            <Limitations limitations={skill.limitations} />
          </section>

          {skill.redistribution?.value === "generated" && (
            <GeneratedNotice skillId={skill.skill_id} versionId={skill.version?.version_id} />
          )}

          <section>
            <h2>它從哪裡來</h2>
            <SkillProvenanceSection skill={skill} />
          </section>

          <VersionHistory skillId={skillId} />

          <SkillIdentifiers skill={skill} />
        </div>
      </div>
    </article>
  );
}
