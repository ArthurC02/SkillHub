import { useEffect, useRef, type RefObject } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Link } from "@tanstack/react-router";
import { ApiError, isWriteResultUnknown } from "../../../../core/api/client";
import { useForkSkill, useOwnCopiesOf, useSkillVersions } from "../../skills.service";
import { SignInAction } from "../../../../shared/ui/SignIn";
import { useUninvited } from "../../../../core/session/me.service";
import type { OwnSkill } from "../../../../core/api/types";
import { Loading } from "../../../../shared/ui/Loading";
import { usePaginationFocus } from "../../../../shared/ui/usePaginationFocus";

const FORK_UNINVITED_REASON_ID = "fork-uninvited-reason";

function forkErrorMessage(error: unknown): string {
  if (isWriteResultUnknown(error))
    return "尚未確認是否已複製到你的工作區。重新送出可能再建立一份副本；請先到資產庫查看，再決定是否重新送出。";
  if (error instanceof ApiError) {
    if (error.status === 403)
      return "這個帳號還沒有封測邀請，所以複製沒有成功。想試的話，用頁尾的「回報問題」選「我想要的東西，這裡沒有」告訴我們你想做什麼。";
    if (error.status === 409)
      return "你的工作區裡這個小工具的副本已經有 10 份，名稱都用完了，所以這次沒有複製；先在資產庫刪掉一份不需要的副本，再複製一次。";
    if (error.status === 404) return "這個小工具已經不在了（可能被下架或刪除），所以無法複製。";
  }
  return "這次複製請求未獲接受，請重新確認小工具資訊後再試。";
}

type ForkContext = { skillId: string; isLoggedIn: boolean; available: boolean };

export function ForkAction({ skillId, isLoggedIn, available }: ForkContext) {
  const lookup = useOwnCopiesOf(skillId, isLoggedIn);
  const { headingRef, loadMore, bindTarget, row } = usePaginationFocus(
    lookup,
    lookup.copies.slice(0, 1),
  );
  return (
    <>
      <h2 id="skill-copy-heading" tabIndex={-1} ref={headingRef}>
        複製一份到你的工作區
      </h2>
      {isLoggedIn ? (
        <ForkControl
          key={skillId}
          skillId={skillId}
          available={available}
          lookup={lookup}
          loadMore={loadMore}
          bindTarget={bindTarget}
          continuationRow={row}
        />
      ) : (
        <div>
          登入後即可把這個小工具複製到你的工作區。 <SignInAction />
        </div>
      )}
    </>
  );
}

function ForkControl({
  skillId,
  available,
  lookup,
  loadMore,
  bindTarget,
  continuationRow,
}: Omit<ForkContext, "isLoggedIn"> & {
  lookup: ReturnType<typeof useOwnCopiesOf>;
  loadMore: () => void;
  bindTarget: (element: HTMLElement | null) => void;
  continuationRow: OwnSkill | undefined;
}) {
  const fork = useForkSkill();
  const versions = useSkillVersions(skillId);
  const cannotPackage = versions.isSuccess && versions.data.versions.length === 0;
  const uncertain = fork.isError && isWriteResultUnknown(fork.error);
  const trigger = useRef<HTMLButtonElement>(null);
  const uninvited = useUninvited();
  const copies = lookup.copies;
  return (
    <div>
      <CopyLookupStatus lookup={lookup} loadMore={loadMore} buttonRef={bindTarget} />
      {copies.length > 0 && !fork.isSuccess && (
        <ExistingCopyHint
          lookup={lookup}
          copyRef={copies[0] === continuationRow ? bindTarget : undefined}
        />
      )}
      <button
        ref={trigger}
        type="button"
        className={cannotPackage ? "action" : undefined}
        onClick={(event) => {
          event.currentTarget.focus();
          fork.mutate(skillId);
        }}
        disabled={uninvited || !available || fork.isPending || fork.isSuccess}
        aria-describedby={uninvited ? FORK_UNINVITED_REASON_ID : undefined}
      >
        {fork.isPending
          ? "建立中…"
          : fork.isSuccess
            ? "已建立自己的版本"
            : uncertain
              ? "重新送出複製請求"
              : "以這個小工具為起點建立我自己的"}
      </button>
      {uninvited && (
        <p className="note" id={FORK_UNINVITED_REASON_ID}>
          平台目前只讓有封測邀請的帳號複製小工具。
        </p>
      )}
      {!available && !fork.isSuccess && (
        <p role="status">小工具資訊尚未重新確認，暫時無法開始複製。</p>
      )}
      {fork.isPending && <p role="status">正在等待建立結果，請保留這一頁以查看回覆。</p>}
      <ForkResult fork={fork} trigger={trigger} />
    </div>
  );
}

function CopyLookupStatus({
  lookup,
  loadMore,
  buttonRef,
}: {
  lookup: ReturnType<typeof useOwnCopiesOf>;
  loadMore: () => void;
  buttonRef: (element: HTMLElement | null) => void;
}) {
  const waiting = lookup.isPending || lookup.fetchStatus !== "idle";
  return (
    <>
      <div id="copy-lookup-progress">
        {waiting && <Loading what="工作區複本清單" paused={lookup.fetchStatus === "paused"} />}
        <ReadFailure
          error={waiting ? undefined : lookup.error}
          what="工作區複本清單"
          retry={() => {
            if (lookup.isFetchNextPageError) loadMore();
            else void lookup.refetch();
          }}
          focusTarget="skill-copy-heading"
        />
        {lookup.partial && (
          <p role="status">
            資產清單尚未讀完（已讀取 {lookup.loaded} / {lookup.total}{" "}
            個小工具），還不能確定全部複本。
          </p>
        )}
      </div>
      {!lookup.error && lookup.data && lookup.hasNextPage && (
        <button
          type="button"
          ref={buttonRef}
          disabled={waiting}
          aria-describedby={waiting ? "copy-lookup-progress" : undefined}
          onClick={loadMore}
        >
          {lookup.fetchStatus === "paused" ? "等待連線…" : waiting ? "查找中…" : "查找更多複本"}
        </button>
      )}
      {lookup.partial && !lookup.hasNextPage && (
        <button
          type="button"
          onClick={() => {
            void lookup.refetch();
            document.getElementById("skill-copy-heading")?.focus();
          }}
        >
          重新確認完整清單
        </button>
      )}
    </>
  );
}

function ExistingCopyHint({
  lookup,
  copyRef,
}: {
  lookup: ReturnType<typeof useOwnCopiesOf>;
  copyRef: ((element: HTMLElement | null) => void) | undefined;
}) {
  const copies = lookup.copies;
  return (
    <p className="note">
      你的工作區已經有這個小工具的複本：
      <Link to="/skills/$skillId" params={{ skillId: copies[0].skill_id }} ref={copyRef}>
        {copies[0].name}
      </Link>
      {copies.length > 1
        ? lookup.complete
          ? `（共 ${copies.length} 份）`
          : `（目前找到 ${copies.length} 份）`
        : ""}
      。要打包、試跑或修改，直接用它；再按一次會另外建立一份。
    </p>
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
          {nameConflict && <Link to="/library">前往資產庫整理副本</Link>}
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
          ，或
          <Link
            to="/skills/$skillId/package"
            params={{ skillId: fork.data.skill_id }}
            search={{ version: fork.data.version_id }}
          >
            直接打包並下載 v{fork.data.version_number}
          </Link>
        </p>
      )}
    </>
  );
}
