import { Link } from "@tanstack/react-router";
import { GeneratedNotice } from "../../components/GeneratedNotice";

export function GenerateSucceeded({
  result,
}: {
  result: { skill_id: string; version_id: string; version_number: number; attempts: number };
}) {
  return (
    <section role="status">
      <h3>已經產生一個 Skill，放在你的工作區</h3>
      <GeneratedNotice skillId={result.skill_id} versionId={result.version_id} />
      {result.attempts > 1 && (
        <p className="note">這一次生成試了 {result.attempts} 趟才通過驗證。</p>
      )}
      <p>
        <Link
          to="/skills/$skillId/versions/$versionId"
          params={{ skillId: result.skill_id, versionId: result.version_id }}
        >
          開啟 v{result.version_number} 並繼續驗證
        </Link>
      </p>
    </section>
  );
}
