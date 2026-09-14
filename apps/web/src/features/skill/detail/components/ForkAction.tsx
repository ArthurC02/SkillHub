import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../../../../core/api/client";
import { useForkSkill, useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";

function forkErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以 Fork 沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409) return "你的工作區已經有同名的 Skill。";
  }
  return "Fork 沒有成功，可以再按一次。";
}

export function ForkAction({ skillId, isLoggedIn }: { skillId: string; isLoggedIn: boolean }) {
  const fork = useForkSkill();
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;

  if (!isLoggedIn) {
    return (
      <div>
        登入後即可 Fork 這個 Skill 到你的工作區。 <SignInAction />
      </div>
    );
  }

  return (
    <div>
      <p className="note">平台目前只讓有封測邀請的帳號 Fork。</p>
      <button
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={() => fork.mutate(skillId)}
        disabled={fork.isPending}
      >
        {fork.isPending ? "建立中…" : "以這個 Skill 為起點建立我自己的"}
      </button>
      {fork.isError && (
        <ReadFailure error={fork.error} what="Fork 這個 Skill">
          <p role="alert">{forkErrorMessage(fork.error)}</p>
        </ReadFailure>
      )}
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
