import { useState } from "react";
import { ApiError, isWriteResultUnknown } from "../../../core/api/client";
import type { OwnSkill, OwnSkillTakedown } from "../../../core/api/types";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { useDeadlinePassed } from "../../../shared/useDeadlinePassed";
import { useRestoreSkill, useTakedownSkill } from "../../skill";

const STALE_STATE = "重新整理清單看看最新狀態。";

function takedownFailureSentence(error: unknown) {
  if (isWriteResultUnknown(error))
    return "無法確認是否已經下架；請先重新整理清單確認，再決定是否重試。";
  if (!(error instanceof ApiError)) return "下架沒有成功，可以再試一次。";
  if (error.status === 400) return "請寫下下架理由。";
  if (error.status === 404) return "找不到這個小工具；它可能已被刪除。";
  if (error.status === 409) return `它已經下架了；${STALE_STATE}`;
  return "下架沒有成功，可以再試一次。";
}

function restoreFailureSentence(error: unknown) {
  if (isWriteResultUnknown(error))
    return "無法確認是否已經恢復上架；請先重新整理清單確認，再決定是否重試。";
  if (!(error instanceof ApiError)) return "恢復上架沒有成功，可以再試一次。";
  if (error.status === 403)
    return "這次下架不能由你恢復：它是平台營運下架的，或已經超過 30 天的恢復期限。";
  if (error.status === 404) return "找不到這個小工具；它可能已被刪除。";
  if (error.status === 409) return `它已經不在下架狀態了；${STALE_STATE}`;
  return "恢復上架沒有成功，可以再試一次。";
}

export function TakedownNotice({
  skill,
  takedown,
  onMessage,
}: {
  skill: OwnSkill;
  takedown: OwnSkillTakedown;
  onMessage: (message: string) => void;
}) {
  const restore = useRestoreSkill();
  const windowClosed = useDeadlinePassed(takedown.restorable_until);
  const restorable = takedown.by === "owner" && !windowClosed;
  return (
    <div className="skill-card-takedown">
      <p>
        <span className="badge badge-danger">已下架</span>{" "}
        {takedown.by === "operator" ? (
          <>
            平台營運在 <Timestamp at={takedown.taken_down_at} /> 下架了它；只有營運者能恢復上架。
          </>
        ) : (
          <>
            你在 <Timestamp at={takedown.taken_down_at} /> 下架了它；
            {restorable ? "恢復期限到" : "恢復期限已在"}{" "}
            <Timestamp at={takedown.restorable_until!} />
            {restorable ? "。" : "結束，不能再恢復上架。"}
          </>
        )}
      </p>
      {restorable && (
        <button
          type="button"
          disabled={restore.isPending}
          onClick={() => {
            onMessage("");
            restore.mutate(skill.skill_id, {
              onSuccess: () =>
                onMessage(
                  `已恢復上架「${skill.name}」；摘要與掃描重新完成後，它才會回到搜尋結果。`,
                ),
            });
          }}
        >
          {restore.isPending ? "恢復中…" : "恢復上架"}
        </button>
      )}
      {restore.error && (
        <ReadFailure error={restore.error} what="恢復上架">
          <p role="alert">{restoreFailureSentence(restore.error)}</p>
        </ReadFailure>
      )}
    </div>
  );
}

export function TakedownMenuItem({
  skill,
  onMessage,
}: {
  skill: OwnSkill;
  onMessage: (message: string) => void;
}) {
  const takedown = useTakedownSkill();
  const [reason, setReason] = useState("");
  const reasonId = `skill-takedown-reason-${skill.skill_id}`;
  const whyId = `skill-takedown-why-${skill.skill_id}`;
  const blank = reason.trim() === "";
  return (
    <li className="skill-menu-takedown">
      <label htmlFor={reasonId}>下架理由</label>
      <textarea
        id={reasonId}
        value={reason}
        readOnly={takedown.isPending}
        onChange={(event) => {
          setReason(event.target.value);
          takedown.reset();
        }}
      />
      {blank && (
        <p className="note" id={whyId}>
          寫下理由才能下架；理由會留在紀錄裡。
        </p>
      )}
      <ConfirmDelete
        scopeId={`skill-takedown-scope-${skill.skill_id}`}
        pending={takedown.isPending}
        disabled={blank}
        disabledReasonId={whyId}
        label="下架"
        confirmLabel="確認下架"
        onAsk={() => {
          onMessage("");
          takedown.reset();
        }}
        onConfirm={() =>
          takedown.mutate(
            { skillId: skill.skill_id, reason: reason.trim() },
            {
              onSuccess: () => onMessage(`已下架「${skill.name}」；30 天內可以在資產庫恢復上架。`),
            },
          )
        }
        scope={
          <>
            下架後它會離開搜尋與公開頁面，詳細頁與打包都停止提供，別人也不能再複製；
            它仍留在你的資產庫，版本、別人已經複製的版本與歷史試跑都不受影響。 30
            天內可以在這裡恢復上架；要讓它離開你的工作區，用刪除。
          </>
        }
      />
      {takedown.error && (
        <ReadFailure error={takedown.error} what="下架">
          <p role="alert">{takedownFailureSentence(takedown.error)}</p>
        </ReadFailure>
      )}
    </li>
  );
}
