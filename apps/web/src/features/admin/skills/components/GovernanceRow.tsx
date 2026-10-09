import { useEffect, useRef } from "react";
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

export function GovernanceRow({
  skill,
  single,
  focusWhenTakenDown,
}: {
  skill: SkillGovernance;
  single: boolean;
  focusWhenTakenDown: boolean;
}) {
  const title = useRef<HTMLElement>(null);
  const previousTakedown = useRef(skill.takedown_at);
  useEffect(() => {
    if (focusWhenTakenDown && !previousTakedown.current && skill.takedown_at)
      title.current?.focus();
    previousTakedown.current = skill.takedown_at;
  }, [focusWhenTakenDown, skill.takedown_at]);

  return (
    <li className="download-item">
      <p>
        <strong ref={title} tabIndex={-1}>
          {skill.name}
        </strong>
      </p>
      <p className="badge-row">
        <span role="status" className={skill.takedown_at ? "badge badge-danger" : "badge"}>
          {skill.takedown_at ? "已下架" : "未下架"}
        </span>{" "}
        <span className={skill.access_restriction ? "badge badge-unverified" : "badge"}>
          {skill.access_restriction
            ? `受限展示：${skill.access_restriction === "license-review" ? "授權審查中" : `其他原因（${skill.access_restriction}）`}`
            : "沒有受限"}
        </span>{" "}
        <span className="badge">
          再散布：{REDISTRIBUTION[skill.redistribution] ?? skill.redistribution}
        </span>{" "}
      </p>
      {skill.takedown_at && (
        <p>
          下架於 <Timestamp at={skill.takedown_at} />
          ，理由：{skill.takedown_reason ?? "未記錄"}。重新上架須先完成審查；後台目前不提供恢復。
        </p>
      )}
      <p className="note">
        小工具 <code>{skill.skill_id}</code>｜工作區 <code>{skill.workspace_id}</code>
      </p>
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
