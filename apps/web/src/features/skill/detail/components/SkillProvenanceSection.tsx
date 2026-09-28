import { Link } from "@tanstack/react-router";
import type { SkillDetail } from "../../../../core/api/types";
import { SourceBlock } from "./SourceBlock";

export function SkillProvenanceSection({ skill }: { skill: SkillDetail }) {
  return (
    <>
      {skill.source ? <SourceBlock source={skill.source} /> : <p>沒有保存任何來源紀錄。</p>}

      <h3>{skill.derivation.label}</h3>
      <p className="note">{skill.derivation.note}</p>
      {skill.derivation.is_fork && skill.derivation.forked_from_skill_id && (
        <p>
          <Link to="/skills/$skillId" params={{ skillId: skill.derivation.forked_from_skill_id }}>
            查看原始 Skill
          </Link>
        </p>
      )}
    </>
  );
}
