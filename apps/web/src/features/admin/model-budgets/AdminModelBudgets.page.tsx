import { useEffect, useRef, useState } from "react";
import { useModelBudgetChange, useModelBudgets, type ModelCallBudget } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { ListFreshness } from "../../../shared/ui/ListFreshness";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";

const CALL_NAMES: Record<string, string> = {
  "match-reasons": "搜尋結果的推薦理由",
  "enrich-skill": "匯入時的小工具摘要增強",
  "generate-skill": "從描述生成小工具",
  "suggest-criteria": "建議驗收條件",
  "judge-run": "評估判定",
  "suggest-improvements": "建議改善",
};

type CompletedChange = {
  kind: string;
  name: string;
  seconds: number | null;
  defaultSeconds: number;
};
type BudgetRowProps = {
  budget: ModelCallBudget;
  onCompleted: (result: CompletedChange) => void;
  onDraftChange: () => void;
};

function BudgetSummary({ budget, name }: { budget: ModelCallBudget; name: string }) {
  return (
    <>
      <p>
        <strong>{name}</strong>
      </p>
      <p className="badge-row">
        <span className={budget.seconds === null ? "badge" : "badge badge-warning"}>
          {budget.seconds === null
            ? `目前：預設 ${budget.default_seconds} 秒`
            : `目前：${budget.seconds} 秒（已調整）`}
        </span>
      </p>
      <p className="note">
        {budget.seconds !== null && `程式預設：${budget.default_seconds} 秒；`}
        可設定範圍：{budget.min_seconds}～{budget.max_seconds} 秒。
      </p>
      {budget.seconds !== null && (
        <>
          <p>理由：{budget.reason}</p>
          {budget.set_at && (
            <p>
              設定於 <Timestamp at={budget.set_at} />
            </p>
          )}
        </>
      )}
    </>
  );
}

function BudgetRow({ budget, onCompleted, onDraftChange }: BudgetRowProps) {
  const name = CALL_NAMES[budget.kind] ?? budget.kind;
  const currentSeconds = budget.seconds ?? budget.default_seconds;
  const [seconds, setSeconds] = useState(String(currentSeconds));
  const [sourceSeconds, setSourceSeconds] = useState(currentSeconds);
  const staleDraft = sourceSeconds !== currentSeconds;
  const editableSeconds = staleDraft ? String(currentSeconds) : seconds;
  const set = useModelBudgetChange("PUT");
  const clear = useModelBudgetChange("DELETE");
  const busy = set.isPending || clear.isPending;
  const wanted = Number(editableSeconds);
  const inRange =
    Number.isInteger(wanted) && wanted >= budget.min_seconds && wanted <= budget.max_seconds;
  const restoreBlockedReason = clear.isSuccess
    ? "這筆恢復請求已完成；請核對上方目前值。"
    : set.isPending
      ? "這一種呼叫正在調整秒數，完成後才能恢復預設。"
      : undefined;
  return (
    <li className="download-item" onInput={onDraftChange}>
      <BudgetSummary budget={budget} name={name} />
      <details id={`admin-budget-${budget.kind}-set`}>
        <summary>調整秒數</summary>
        {staleDraft && (
          <p role="status">設定已變更；草稿改為最新的 {currentSeconds} 秒，請確認後再送出。</p>
        )}
        <ActionForm
          id={`admin-budget-${budget.kind}`}
          submitLabel={`改 ${name} 的秒數`}
          pending={set.isPending}
          error={set.error}
          ready={inRange && !clear.isPending}
          blockedReason={
            clear.isPending ? "這一種呼叫正在恢復預設，完成後才能調整秒數。" : undefined
          }
          done={set.isSuccess && "調整請求已完成；請核對上方的目前值。"}
          contextKey={`${budget.kind}:${editableSeconds}`}
          onSubmit={(reason) => {
            clear.reset();
            set.mutate(
              { kind: budget.kind, seconds: wanted, reason },
              {
                onSuccess: () => {
                  setSourceSeconds(wanted);
                  onCompleted({
                    kind: budget.kind,
                    name,
                    seconds: wanted,
                    defaultSeconds: budget.default_seconds,
                  });
                },
              },
            );
          }}
        >
          <div className="field">
            <label htmlFor={`admin-budget-${budget.kind}-seconds`}>
              秒數（{budget.min_seconds}～{budget.max_seconds}）
            </label>
            <input
              id={`admin-budget-${budget.kind}-seconds`}
              inputMode="numeric"
              value={editableSeconds}
              onChange={(event) => {
                setSourceSeconds(currentSeconds);
                setSeconds(event.target.value);
                set.reset();
              }}
              readOnly={busy}
              aria-describedby={inRange ? undefined : `admin-budget-${budget.kind}-range`}
            />
            {!inRange && (
              <p id={`admin-budget-${budget.kind}-range`} className="note">
                要填 {budget.min_seconds} 到 {budget.max_seconds} 之間的整數秒。上限是程式裡的
                deadline 扣掉安全邊界，超過它平台會比模型服務先放棄等待。
              </p>
            )}
          </div>
        </ActionForm>
      </details>
      {budget.seconds !== null && (
        <details id={`admin-budget-${budget.kind}-clear`}>
          <summary>恢復程式預設（{budget.default_seconds} 秒）</summary>
          <ActionForm
            id={`admin-budget-${budget.kind}-clear`}
            submitLabel={`把 ${name} 改回預設`}
            pending={clear.isPending}
            error={clear.error}
            ready={!set.isPending && !clear.isSuccess}
            blockedReason={restoreBlockedReason}
            onSubmit={(reason) => {
              set.reset();
              clear.mutate(
                { kind: budget.kind, reason },
                {
                  onSuccess: () => {
                    setSeconds(String(budget.default_seconds));
                    setSourceSeconds(budget.default_seconds);
                    onCompleted({
                      kind: budget.kind,
                      name,
                      seconds: null,
                      defaultSeconds: budget.default_seconds,
                    });
                  },
                },
              );
            }}
          />
        </details>
      )}
    </li>
  );
}

function completedMessage(
  completed: CompletedChange,
  current: ModelCallBudget | undefined,
  fetching: boolean,
  error: unknown,
) {
  if (fetching) return "正在重新讀取目前設定。";
  if (error) return "目前設定暫時無法重新讀取，請稍後核對。";
  if (current?.seconds === completed.seconds) {
    return `目前顯示${completed.seconds === null ? "程式預設" : "設定"} ${completed.seconds ?? completed.defaultSeconds} 秒；下次呼叫平台會送出此值，模型服務可能採用更短的上限。`;
  }
  if (current) {
    return `重新讀取仍顯示 ${current.seconds === null ? `程式預設 ${current.default_seconds}` : current.seconds} 秒；請重新整理確認。`;
  }
  return "重新讀取未找到這種模型呼叫；請重新整理確認。";
}

export function AdminModelBudgets() {
  const budgets = useModelBudgets();
  const [completed, setCompleted] = useState<CompletedChange | null>(null);
  const result = useRef<HTMLParagraphElement>(null);
  const current = budgets.data?.budgets.find((budget) => budget.kind === completed?.kind);
  const verified =
    completed && !budgets.isFetching && !budgets.error && current?.seconds === completed.seconds;

  useEffect(() => {
    if (completed) result.current?.focus();
  }, [completed]);

  return (
    <AdminPage heading="模型呼叫逾時">
      <p className="note">
        每一種模型呼叫最多可以跑多久。改了之後平台會把這個秒數一起送給模型服務，模型服務只會採用比它自己上限更短的那一個。
        上限由程式決定，這裡只能在上限以內調整。
      </p>
      {budgets.isPending && <Loading what="模型呼叫逾時" />}
      <ReadFailure error={budgets.error} what="模型呼叫逾時">
        <p role="alert">暫時無法讀取模型呼叫逾時。</p>
        <button type="button" disabled={budgets.isFetching} onClick={() => void budgets.refetch()}>
          {budgets.isFetching ? "重新讀取中…" : "再試一次"}
        </button>
      </ReadFailure>
      {completed && (
        <p
          id="admin-budget-result"
          ref={result}
          tabIndex={-1}
          className={`notice ${verified ? "notice-success" : "notice-warning"}`}
          role="status"
        >
          {completed.name}的{completed.seconds === null ? "恢復預設" : "調整秒數"}請求已完成。
          {completedMessage(completed, current, budgets.isFetching, budgets.error)}
        </p>
      )}
      {budgets.data && !budgets.error && (
        <>
          <ListFreshness
            inFlight={false}
            showWhenIdle
            updatedAt={budgets.dataUpdatedAt}
            fetching={budgets.isFetching}
            refetch={budgets.refetch}
            subject="逾時設定"
          />
          {budgets.data.budgets.length === 0 ? (
            <p>目前沒有可設定的模型呼叫。</p>
          ) : (
            <ul className="download-list">
              {budgets.data.budgets.map((budget) => (
                <BudgetRow
                  budget={budget}
                  key={budget.kind}
                  onCompleted={setCompleted}
                  onDraftChange={() => setCompleted(null)}
                />
              ))}
            </ul>
          )}
        </>
      )}
    </AdminPage>
  );
}
