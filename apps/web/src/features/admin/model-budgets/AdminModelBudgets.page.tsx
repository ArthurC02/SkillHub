import { useState } from "react";
import { useModelBudgetChange, useModelBudgets, type ModelCallBudget } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
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

function unavailableReason(fresh: boolean, otherPending: boolean, pendingReason: string) {
  if (!fresh) return "正在確認最新設定，完成後才能更改。";
  return otherPending ? pendingReason : undefined;
}

function editableSeconds(budget: ModelCallBudget, edit: { from: string; value: string } | null) {
  return edit?.from === JSON.stringify(budget)
    ? edit.value
    : String(budget.seconds ?? budget.default_seconds);
}

function BudgetRow({ budget, fresh }: { budget: ModelCallBudget; fresh: boolean }) {
  const name = CALL_NAMES[budget.kind] ?? budget.kind;
  const [edit, setEdit] = useState<{ from: string; value: string } | null>(null);
  const from = JSON.stringify(budget);
  const seconds = editableSeconds(budget, edit);
  const set = useModelBudgetChange("PUT");
  const clear = useModelBudgetChange("DELETE");
  const changing = set.isPending || clear.isPending;
  const wanted = Number(seconds);
  const inRange =
    Number.isInteger(wanted) && wanted >= budget.min_seconds && wanted <= budget.max_seconds;

  return (
    <li className="download-item">
      <p>
        <strong>{name}</strong>
      </p>
      <p className="badge-row">
        <span className="badge">程式預設 {budget.default_seconds} 秒</span>
        {budget.seconds !== null && <span className="badge">管理員設定 {budget.seconds} 秒</span>}
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
      {clear.isSuccess && budget.seconds === null && (
        <p className="notice notice-success" role="status">
          已改回預設。
        </p>
      )}
      <ActionForm
        id={`admin-budget-${budget.kind}`}
        submitLabel={`改 ${name} 的秒數`}
        pending={set.isPending}
        error={set.error}
        ready={fresh && inRange && !clear.isPending}
        unavailableReason={unavailableReason(
          fresh,
          clear.isPending,
          "此呼叫正在改回預設，完成後才能再次設定。",
        )}
        done={set.isSuccess && "已套用，下一次呼叫就用這個秒數。"}
        contextKey={`${budget.kind}:${seconds}`}
        onSubmit={(reason) => {
          clear.reset();
          set.mutate({ kind: budget.kind, seconds: wanted, reason });
        }}
      >
        <div className="field">
          <label htmlFor={`admin-budget-${budget.kind}-seconds`}>
            秒數（{budget.min_seconds}～{budget.max_seconds}）
          </label>
          <input
            id={`admin-budget-${budget.kind}-seconds`}
            inputMode="numeric"
            value={seconds}
            onChange={(event) => {
              setEdit({ from, value: event.target.value });
              set.reset();
              clear.reset();
            }}
            readOnly={changing || !fresh}
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
      {budget.seconds !== null && (
        <ActionForm
          id={`admin-budget-${budget.kind}-clear`}
          submitLabel={`把 ${name} 改回預設`}
          pending={clear.isPending}
          error={clear.error}
          ready={fresh && !set.isPending}
          unavailableReason={unavailableReason(
            fresh,
            set.isPending,
            "此呼叫正在設定秒數，完成後才能改回預設。",
          )}
          onSubmit={(reason) => {
            set.reset();
            clear.mutate({ kind: budget.kind, reason });
          }}
        />
      )}
    </li>
  );
}

export function AdminModelBudgets() {
  const budgets = useModelBudgets();

  return (
    <AdminPage heading="模型呼叫逾時">
      <p className="note">
        每一種模型呼叫最多可以跑多久。改了之後平台會把這個秒數一起送給模型服務，模型服務只會採用比它自己上限更短的那一個。
        上限由程式決定，這裡只能在上限以內調整。
      </p>
      {budgets.isPending && <Loading what="模型呼叫逾時" />}
      {budgets.isFetching && !budgets.isPending && (
        <p className="note" role="status">
          正在確認最新設定；完成前不能更改。
        </p>
      )}
      <ReadFailure
        error={budgets.error}
        what="模型呼叫逾時"
        onRetry={() => void budgets.refetch()}
        retrying={budgets.isFetching}
      />
      {budgets.data && !budgets.error && (
        <ul className="download-list">
          {budgets.data.budgets.map((budget) => (
            <BudgetRow budget={budget} fresh={!budgets.isFetching} key={budget.kind} />
          ))}
        </ul>
      )}
    </AdminPage>
  );
}
