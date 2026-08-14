import { Link, useParams } from "@tanstack/react-router";
import { useForkSkill, useSkillDetail } from "../api/skills";
import { useMe } from "../api/me";
import { CompatibilityStatus } from "../components/CompatibilityStatus";
import { LicenseBadge } from "../components/LicenseBadge";
import { RiskIndicator } from "../components/RiskIndicator";
import { TrustBadge } from "../components/TrustBadge";
import { VerificationStatus } from "../components/VerificationStatus";

// DISC-006: Skill general detail page.
export function SkillDetail() {
  const { skillId } = useParams({ from: "/skills/$skillId" });
  const { data: skill, isLoading, isError } = useSkillDetail(skillId);
  const { data: me } = useMe();

  if (isLoading) return <p>載入中…</p>;
  if (isError || !skill) return <p role="alert">找不到這個 Skill，或載入失敗。</p>;

  return (
    <article>
      <header>
        <h1>{skill.name}</h1>
        <p>{skill.summary}</p>
        <div className="badge-row">
          <TrustBadge level={skill.source.trust_level} />
          <LicenseBadge status={skill.license.status} name={skill.license.name} />
          <VerificationStatus verification={skill.verification} />
        </div>
      </header>

      {skill.forked_from_skill_id && (
        <p>
          Fork 自{" "}
          <Link to="/skills/$skillId" params={{ skillId: skill.forked_from_skill_id }}>
            原始 Skill
          </Link>
        </p>
      )}

      <section>
        <h2>功能</h2>
        <ul>
          {skill.capabilities.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>限制</h2>
        <ul>
          {skill.limitations.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>輸入</h2>
        <ul>
          {skill.inputs.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>輸出</h2>
        <ul>
          {skill.outputs.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>依賴</h2>
        <ul>
          {skill.dependencies.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>所需權限</h2>
        <ul>
          {skill.required_permissions.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>

      <section>
        <h2>來源與 License</h2>
        <p>
          信任層級：<TrustBadge level={skill.source.trust_level} />
        </p>
        {skill.source.url && (
          <p>
            來源網址：<a href={skill.source.url}>{skill.source.url}</a>
          </p>
        )}
        {skill.source.source_version && <p>來源版本／Commit：{skill.source.source_version}</p>}
        {skill.source.fetched_at && <p>擷取時間：{skill.source.fetched_at}</p>}
        {skill.source.content_hash && (
          <p>
            內容雜湊：<code>{skill.source.content_hash}</code>
          </p>
        )}
        <p>
          License：<LicenseBadge status={skill.license.status} name={skill.license.name} />
        </p>
      </section>

      <section>
        <h2>相容性</h2>
        <CompatibilityStatus entries={skill.compatibility} />
      </section>

      <section>
        <h2>風險提示</h2>
        <RiskIndicator risk={skill.risk} />
      </section>

      <nav>
        <Link to="/skills/$skillId/files" params={{ skillId }}>
          查看 SKILL.md 與檔案樹（進階模式）
        </Link>
      </nav>

      <ForkAction skillId={skillId} isLoggedIn={!!me} />
    </article>
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
