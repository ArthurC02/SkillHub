import { Link } from "@tanstack/react-router";
import { useId } from "react";
import { NavScrollCue } from "../../../shared/ui/NavScrollCue";

export function SkillWorkspaceNav({
  skillId,
  versionId,
  testCaseId,
}: {
  skillId: string;
  versionId?: string;
  testCaseId?: string;
}) {
  const versionUnavailableReasonId = useId();

  return (
    <>
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
        {testCaseId ? (
          <Link
            to="/lab/test-cases/$testCaseId"
            params={{ testCaseId }}
            search={{ version: versionId }}
            className="chip"
          >
            驗證
          </Link>
        ) : (
          <Link
            to="/lab/test-cases"
            search={{ skill: skillId, version: versionId }}
            className="chip"
          >
            驗證
          </Link>
        )}
        {versionId ? (
          <Link
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId, versionId }}
            className="chip"
          >
            版本與發佈
          </Link>
        ) : (
          <button
            type="button"
            className="chip"
            disabled
            aria-describedby={versionUnavailableReasonId}
          >
            版本與發佈
          </button>
        )}
        <NavScrollCue />
      </nav>
      {!versionId && (
        <p id={versionUnavailableReasonId} className="note">
          先選定一個版本，才能查看這一版的驗證、打包與發佈脈絡。
        </p>
      )}
    </>
  );
}
