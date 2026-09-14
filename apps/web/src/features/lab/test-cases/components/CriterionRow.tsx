import { formatAt } from "../../../../shared/ui/Timestamp";
import { useState } from "react";
import { useCriterionAction } from "../../testcases.service";
import { ConfirmDelete } from "../../../../shared/ui/ConfirmDelete";
import type { AcceptanceCriterion } from "../../testcases.service";
import { MutationError } from "./MutationError";

function criterionState(c: AcceptanceCriterion): string {
  if (c.confirmed_at) return `已確認（${formatAt(c.confirmed_at)}）`;
  return c.source === "suggested" ? "系統建議，尚未確認" : "尚未確認";
}

export function CriterionRow({
  testCaseId,
  criterion,
}: {
  testCaseId: string;
  criterion: AcceptanceCriterion;
}) {
  const [draft, setDraft] = useState(criterion.text);
  const action = useCriterionAction(testCaseId, criterion.id);
  const running = action.isPending ? action.variables?.kind : undefined;

  const edited = draft !== criterion.text;
  const saveReason = !edited ? "沒有變更要存" : draft.trim() === "" ? "驗收條件不能是空白" : null;

  return (
    <li className="criterion">
      <label htmlFor={`criterion-${criterion.id}`} className="note">
        驗收條件
      </label>{" "}
      <input
        id={`criterion-${criterion.id}`}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        size={50}
        maxLength={2000}
      />
      <p className="note">狀態：{criterionState(criterion)}</p>
      {edited && (
        <p className="note" id={`criterion-edited-${criterion.id}`}>
          {criterion.confirmed_at
            ? "改動文字後儲存會清除這一條的確認，因為當初確認的是舊的文字。"
            : "文字改了還沒儲存，所以現在不能確認——確認的必須是已經存下來的那一句。先按「儲存文字」。"}
        </p>
      )}
      <p>
        <button
          type="button"
          disabled={action.isPending || !edited || draft.trim() === ""}
          aria-describedby={saveReason ? `criterion-save-${criterion.id}` : undefined}
          onClick={() => action.mutate({ kind: "save", text: draft })}
        >
          {running ? (running === "save" ? "儲存中…" : "儲存文字（另一項處理中…）") : "儲存文字"}
        </button>{" "}
        {criterion.confirmed_at ? (
          <button
            type="button"
            disabled={action.isPending}
            onClick={() => action.mutate({ kind: "unconfirm" })}
          >
            {running
              ? running === "unconfirm"
                ? "取消確認中…"
                : "取消確認（另一項處理中…）"
              : "取消確認"}
          </button>
        ) : (
          <button
            type="button"
            disabled={action.isPending || edited}
            aria-describedby={edited ? `criterion-edited-${criterion.id}` : undefined}
            onClick={() => action.mutate({ kind: "confirm" })}
          >
            {running ? (running === "confirm" ? "確認中…" : "確認（另一項處理中…）") : "確認"}
          </button>
        )}{" "}
        <ConfirmDelete
          scopeId={`criterion-delete-scope-${criterion.id}`}
          scope={
            <>
              會刪掉這一條驗收條件的文字與它的確認狀態，之後的 Run
              不再逐條判定它。這一條沒有暫存區也沒有保留期，按下去就沒有了，救不回來。 已經跑過的
              Run 與已經寫好的評估不受影響——它們判定的是當時凍結的快照，那一份仍然 留著這一條。
              掛在這一條上的 rubric 說明要另外處理，在下面的「Rubric（選用）」那一節。
            </>
          }
          pending={action.isPending}
          onAsk={() => action.reset()}
          onConfirm={() => action.mutate({ kind: "delete" })}
          label="刪除這一條"
          confirmLabel="確認刪除這一條"
        />
      </p>
      {saveReason && (
        <p className="note" role="status" id={`criterion-save-${criterion.id}`}>
          {saveReason}
        </p>
      )}
      <MutationError
        error={action.error}
        what="這一條驗收條件"
        fallback="操作沒有成功，可以再試一次。"
      />
    </li>
  );
}
