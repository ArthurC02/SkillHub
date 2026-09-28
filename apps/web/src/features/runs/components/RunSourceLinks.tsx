import { Link } from "@tanstack/react-router";
import type { RunListItem } from "../runs.service";

export function RunSourceLinks({ run }: { run: RunListItem }) {
  return (
    <>
      <Link to="/skills/$skillId" params={{ skillId: run.skill_id }}>
        <strong>{run.skill_name}</strong>
      </Link>
      {" · "}
      <Link
        to="/skills/$skillId/versions/$versionId"
        params={{ skillId: run.skill_id, versionId: run.skill_version_id }}
      >
        這次的版本
      </Link>
      {run.test_case_id && (
        <>
          {" · "}
          <Link
            to="/lab/test-cases/$testCaseId"
            params={{ testCaseId: run.test_case_id }}
            search={{ version: run.skill_version_id }}
          >
            Test Case
          </Link>
        </>
      )}
    </>
  );
}
