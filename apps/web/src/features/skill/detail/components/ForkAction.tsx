import { useEffect, useRef, type RefObject } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { ApiError, isWriteResultUnknown } from "../../../../core/api/client";
import { useForkSkill, useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";

function forkErrorMessage(error: unknown): string {
  if (isWriteResultUnknown(error))
    return "尚未確認是否已複製到你的工作區。重新送出可能再建立一份副本；請先到資產庫查看，再決定是否重新送出。";
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以複製沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409) return "你的工作區已經有同名的小工具。";
  }
  return "這次複製請求未獲接受，請重新確認小工具資訊後再試。";
}

type ForkContext = { skillId: string; isLoggedIn: boolean; available: boolean };

export function ForkAction({ skillId, isLoggedIn, available }: ForkContext) {
  if (!isLoggedIn) {
    return (
      <div>
        登入後即可把這個小工具複製到你的工作區。 <SignInAction />
      </div>
    );
  }
  return <ForkControl key={skillId} skillId={skillId} available={available} />;
}

function ForkControl({ skillId, available }: Omit<ForkContext, "isLoggedIn">) {
  const fork = useForkSkill();
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;
  const uncertain = fork.isError && isWriteResultUnknown(fork.error);
  const trigger = useRef<HTMLButtonElement>(null);
  return (
    <div>
      <p className="note">平台目前只讓有封測邀請的帳號複製小工具。</p>
      <button
        ref={trigger}
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={(event) => {
          event.currentTarget.focus();
          fork.mutate(skillId);
        }}
        disabled={!available || fork.isPending || fork.isSuccess}
      >
        {fork.isPending
          ? "建立中…"
          : fork.isSuccess
            ? "已建立自己的版本"
            : uncertain
              ? "重新送出複製請求"
              : "以這個小工具為起點建立我自己的"}
      </button>
      {!available && !fork.isSuccess && (
        <p role="status">小工具資訊尚未重新確認，暫時無法開始複製。</p>
      )}
      {fork.isPending && <p role="status">正在等待建立結果，請保留這一頁以查看回覆。</p>}
      <ForkResult fork={fork} trigger={trigger} />
    </div>
  );
}
function ForkResult({
  fork,
  trigger,
}: {
  fork: ReturnType<typeof useForkSkill>;
  trigger: RefObject<HTMLButtonElement | null>;
}) {
  const nameConflict = fork.error instanceof ApiError && fork.error.status === 409;
  const uncertain = fork.isError && isWriteResultUnknown(fork.error);
  const receipt = useRef<HTMLAnchorElement>(null);
  const failure = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    const result = fork.isSuccess ? receipt.current : fork.isError ? failure.current : null;
    if (
      result &&
      (document.activeElement === trigger.current || document.activeElement === document.body)
    ) {
      result.focus();
      result.scrollIntoView({ block: "center" });
    }
  }, [fork.isSuccess, fork.isError, trigger]);
  return (
    <>
      {fork.isError && (
        <ReadFailure error={fork.error} what="複製小工具">
          <p ref={failure} tabIndex={-1} role="alert" aria-label="複製小工具結果">
            {forkErrorMessage(fork.error)}
          </p>
          {uncertain && (
            <Link to="/library" target="_blank" rel="noopener">
              另開資產庫確認複製結果
            </Link>
          )}
          {nameConflict && <Link to="/library">前往資產庫找出同名小工具</Link>}
        </ReadFailure>
      )}
      {fork.isSuccess && (
        <p role="status">
          已複製到你的工作區：
          <Link
            ref={receipt}
            to="/skills/$skillId/versions/$versionId"
            params={{ skillId: fork.data.skill_id, versionId: fork.data.version_id }}
          >
            開啟 {fork.data.name} v{fork.data.version_number} 並繼續驗證
          </Link>
        </p>
      )}
    </>
  );
}
