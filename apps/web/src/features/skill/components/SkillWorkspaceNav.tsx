import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { NavScrollCue } from "../../../shared/ui/NavScrollCue";

export function SkillWorkspaceNav({
  skillId,
  versionId,
  canPackage = false,
}: {
  skillId: string;
  versionId?: string;
  canPackage?: boolean;
}) {
  const packageUnavailableReasonId = useId();

  return (
    <nav aria-label="這個 Skill 的工作台" className="category-nav">
      <Link
        to="/skills/$skillId"
        params={{ skillId }}
        className="chip"
        activeOptions={{ exact: true }}
      >
        總覽
      </Link>
      <Link to="/skills/$skillId/files" params={{ skillId }} className="chip">
        檔案
      </Link>
      <Link to="/lab/test-cases" search={{ skill: skillId }} className="chip">
        驗證
      </Link>
      {canPackage ? (
        <Link
          to="/skills/$skillId/package"
          params={{ skillId }}
          search={{ version: versionId }}
          className="chip"
        >
          打包
        </Link>
      ) : (
        <>
          <button
            type="button"
            className="chip"
            disabled
            aria-describedby={packageUnavailableReasonId}
          >
            打包
          </button>
          <span id={packageUnavailableReasonId} hidden>
            只有自己工作區裡、通過可散布檢查的版本才可打包。
          </span>
        </>
      )}
      <NavScrollCue />
    </nav>
  );
}
