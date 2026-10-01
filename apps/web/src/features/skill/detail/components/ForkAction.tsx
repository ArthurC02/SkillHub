import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../../../../core/api/client";
import { useForkSkill, useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";

function forkErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以複製沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409) return "你的工作區已經有同名的小工具。";
  }
  return "複製沒有成功，可以再按一次。";
}

export function ForkAction({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const fork = useForkSkill();
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;
  const nameConflict = fork.error instanceof ApiError && fork.error.status === 409;

  if (!isLoggedIn) {
    return (
      <div>
        登入後即可把這個小工具複製到你的工作區。 <SignInAction />
      </div>
    );
  }

  return (
    <div>
      <p className="note">平台目前只讓有封測邀請的帳號複製小工具。</p>
      <button
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={() => fork.mutate(skillId)}
        disabled={fork.isPending || fork.isSuccess}
      >
        {fork.isPending
          ? "建立中…"
          : fork.isSuccess
            ? "已建立自己的版本"
            : "以這個小工具為起點建立我自己的"}
      </button>
      {fork.isError && (
        <ReadFailure error={fork.error} what="複製小工具">
          <p role="alert">{forkErrorMessage(fork.error)}</p>
          {nameConflict && <Link to="/library">前往資產庫找出同名小工具</Link>}
        </ReadFailure>
      )}
      {fork.isSuccess && (
        <p role="status">
          已複製到你的工作區：
          <Link
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId: fork.data.skill_id, versionId: fork.data.version_id }}
          >
            開啟 {fork.data.name} v{fork.data.version_number} 並繼續驗證
          </Link>
        </p>
      )}
    </div>
  );
}
