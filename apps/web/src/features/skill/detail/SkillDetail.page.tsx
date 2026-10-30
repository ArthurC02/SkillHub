import type { SkillDetail as SkillDetailData } from "../../../core/api/types";
import { Loading } from "../../../shared/ui/Loading";
import { PageReadFailure } from "../../../shared/ui/LoginRequired";
import { Link, useParams } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import { unreadableSkillSentence, useSkillDetail } from "../skills.service";
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
  const detail = useSkillDetail(skillId);
  const { data: me } = useMe();
  const skill = detail.error ? undefined : detail.data;
  const retainFork = !(
    detail.error instanceof ApiError && [401, 403, 404, 410].includes(detail.error.status)
  );
  return (
    <article className="skill-detail">
      {skill ? (
        <SkillDetailHeader
          skillId={skillId}
          skill={skill}
          refreshing={detail.fetchStatus !== "idle"}
        />
      ) : (
        <SkillDetailReadState detail={detail} />
      )}
      {detail.data && (
        <div className="detail-layout">
          <aside className="detail-rail" aria-label="這個小工具的操作">
            {skill && <TrialEntry skillId={skillId} isLoggedIn={!!me} />}
            {retainFork && (
              <section>
                <ForkAction
                  key={skillId}
                  skillId={skillId}
                  isLoggedIn={!!me}
                  available={Boolean(skill) && detail.fetchStatus === "idle"}
                />
              </section>
            )}
            {skill?.version && !skill.access_restriction && (
              <nav>
                <Link to="/skills/$skillId/files" params={{ skillId }}>
                  查看 SKILL.md 與檔案樹（進階模式）
                </Link>
              </nav>
            )}
            <CategoryEditor skillId={skillId} skill={detail} />
          </aside>
          {skill && <SkillDetailFacts skillId={skillId} skill={skill} isLoggedIn={!!me} />}
        </div>
      )}
    </article>
  );
}

function SkillDetailReadState({ detail }: { detail: ReturnType<typeof useSkillDetail> }) {
  const { data: skill, isLoading, isPending, error, refetch, fetchStatus } = detail;
  if (isLoading || isPending)
    return (
      <section>
        <h1>小工具總覽</h1>
        <Loading what="這個小工具" />
      </section>
    );
  const unreadable = unreadableSkillSentence(error);
  if (unreadable)
    return (
      <PageReadFailure heading="小工具總覽" error={error} what="這個小工具">
        <p role="alert">{unreadable}</p>
      </PageReadFailure>
    );
  if (error)
    return (
      <PageReadFailure
        heading="小工具總覽"
        error={error}
        what="這個小工具"
        retry={() => void refetch()}
        retrying={fetchStatus !== "idle"}
      />
    );
  if (!skill)
    return (
      <section>
        <h1>小工具總覽</h1>
        <p role="alert">找不到這個小工具。</p>
      </section>
    );
  return null;
}

type SkillDetailContent = { skillId: string; skill: SkillDetailData; isLoggedIn: boolean };

function SkillDetailHeader({
  skillId,
  skill,
  refreshing,
}: Omit<SkillDetailContent, "isLoggedIn"> & { refreshing: boolean }) {
  return (
    <header className="detail-answer">
      <h1>{skill.name}</h1>
      <p>{skill.summary}</p>
      <div className="badge-row">
        <LabelledBadge kind="category" value={skill.category} />
        <LabelledBadge kind="tier" value={skill.tier} />
        {skill.source && <LabelledBadge kind="trust" value={skill.source.trust} />}
      </div>
      {refreshing && <p role="status">小工具資訊正在重新確認，以下為上次讀到的內容。</p>}
      <SkillWorkspaceNav skillId={skillId} versionId={skill.version?.version_id} />
    </header>
  );
}

function SkillDetailFacts({ skillId, skill, isLoggedIn }: SkillDetailContent) {
  return (
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

        <Redistribution skill={skill} isLoggedIn={isLoggedIn} />

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
  );
}
