import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";
import type { SkillDetail as SkillDetailModel } from "../../../../core/api/types";

export function PackagingEntry({
  skill,
  isLoggedIn,
}: {
  skill: SkillDetailModel;
  isLoggedIn: boolean;
}) {
  const versions = useSkillVersions(skill.skill_id);

  if (!isLoggedIn)
    return (
      <div className="note">
        打包與下載需要登入，而且只打包得了你自己工作區裡的版本——別人的 Skill 要先 Fork 一份。{" "}
        <SignInAction />
      </div>
    );
  if (versions.isPending) return <Loading what="這個 Skill 在你工作區的版本" />;
  if (versions.error) return <ReadFailure error={versions.error} what="這個 Skill 的版本" />;
  if ((versions.data?.versions.length ?? 0) === 0)
    return (
      <p className="note">
        這個 Skill 不在你的工作區，所以沒有屬於你的版本可以打包。
        <strong>要先 Fork 一份</strong>——旁邊的「Fork 到你的工作區」就是那一步。
      </p>
    );

  return (
    <p>
      <Link
        className="action"
        to="/skills/$skillId/package"
        params={{ skillId: skill.skill_id }}
        search={{ version: skill.version!.version_id }}
      >
        打包並下載這個版本
      </Link>
    </p>
  );
}
