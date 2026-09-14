import { Link } from "@tanstack/react-router";
import type { SkillGovernance } from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";

const REDISTRIBUTION: Record<string, string> = {
  allowed: "可以再散布",
  blocked: "禁止再散布",
  unknown: "尚未判定",
  self_supplied: "使用者自己提供",
  generated: "平台生成",
};

export function GovernanceRow({ skill, single }: { skill: SkillGovernance; single: boolean }) {
  return (
    <li className="download-item">
      <p>
        <strong>{skill.name}</strong>
      </p>
      <p className="note">
        Skill <code>{skill.skill_id}</code>｜工作區 <code>{skill.workspace_id}</code>
      </p>
      <p className="badge-row">
        <span className={skill.access_restriction ? "badge badge-unverified" : "badge"}>
          {skill.access_restriction ? `受限展示：${skill.access_restriction}` : "沒有受限"}
        </span>{" "}
        <span className="badge">
          再散布：{REDISTRIBUTION[skill.redistribution] ?? skill.redistribution}
        </span>{" "}
        {skill.takedown_at && <span className="badge badge-danger">已下架</span>}
      </p>
      {skill.takedown_at && (
        <p>
          下架於 <Timestamp at={skill.takedown_at} />
          ，理由：{skill.takedown_reason ?? "未記錄"}。下架沒有恢復的路。
        </p>
      )}
      {!single && (
        <p>
          <Link to="/admin/skills" search={{ q: skill.skill_id }}>
            處理這一個
          </Link>
        </p>
      )}
    </li>
  );
}
