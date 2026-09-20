import { useState } from "react";
import { useModelBudgetChange, useModelBudgets, type ModelCallBudget } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";

const CALL_NAMES: Record<string, string> = {
  "match-reasons": "搜尋結果的推薦理由",
  "enrich-skill": "匯入時的 Skill 摘要增強",
  "generate-skill": "從描述生成 Skill",
  "suggest-criteria": "建議驗收條件",
  "judge-run": "評估判定",
  "suggest-improvements": "建議改善",
};

function BudgetRow({ budget }: { budget: ModelCallBudget }) {
  const name = CALL_NAMES[budget.kind] ?? budget.kind;
  const [seconds, setSeconds] = useState(String(budget.seconds ?? budget.default_seconds));
  const set = useModelBudgetChange("PUT");
  const clear = useModelBudgetChange("DELETE");
  const wanted = Number(seconds);
  const inRange =
    Number.isInteger(wanted) && wanted >= budget.min_seconds && wanted <= budget.max_seconds;

  return (
    <li className="download-item">
      <p>
        <strong>{name}</strong>
      </p>
      <p className="badge-row">
        <span className={budget.seconds === null ? "badge" : "badge badge-danger"}>
          {budget.seconds === null ? `預設 ${budget.default_seconds} 秒` : `${budget.seconds} 秒`}
        </span>
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
      <ActionForm
        id={`admin-budget-${budget.kind}`}
        submitLabel={`改 ${name} 的秒數`}
        pending={set.isPending}
        error={set.error}
        ready={inRange}
        done={set.isSuccess && "已套用，下一次呼叫就用這個秒數。"}
        onSubmit={(reason) => set.mutate({ kind: budget.kind, seconds: wanted, reason })}
      >
        <div className="field">
          <label htmlFor={`admin-budget-${budget.kind}-seconds`}>
            秒數（{budget.min_seconds}～{budget.max_seconds}）
          </label>
          <input
            id={`admin-budget-${budget.kind}-seconds`}
            inputMode="numeric"
            value={seconds}
            onChange={(event) => setSeconds(event.target.value)}
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
          done={clear.isSuccess && "已改回預設。"}
          onSubmit={(reason) => clear.mutate({ kind: budget.kind, reason })}
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
      <ReadFailure error={budgets.error} what="模型呼叫逾時" />
      {budgets.data && (
        <ul className="download-list">
          {budgets.data.budgets.map((budget) => (
            <BudgetRow budget={budget} key={budget.kind} />
          ))}
        </ul>
      )}
    </AdminPage>
  );
}
