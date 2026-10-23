import { useState } from "react";
import { useRunModelBudget, type RunSettings } from "../admin.service";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";
import { SettingsResult } from "./SettingsResult";

type Budget = RunSettings["model_budget"];

const dollars = (usd: number) => `US$${usd.toFixed(2)}`;

function BudgetFacts({ setting }: { setting: Budget }) {
  return (
    <>
      <p>
        目前：每次試跑最多花 <strong>{dollars(setting.usd)}</strong> 的模型費用
        {setting.reason ? `（部署預設 ${dollars(setting.default_usd)}）` : "（部署預設）"}
      </p>
      <p className="note">
        開始試跑前先確認點數夠付這個上限，試跑的模型金鑰也花不超過它；實際扣點仍照實付。
        新建立的試跑就照新的上限，已經建立的試跑維持建立當時的上限。
      </p>
      {setting.reason && (
        <>
          <p>理由：{setting.reason}</p>
          {setting.set_at && (
            <p>
              設定於 <Timestamp at={setting.set_at} />
            </p>
          )}
        </>
      )}
    </>
  );
}

function resultSentence(usd: number | undefined, defaultUSD: number) {
  return usd === undefined
    ? `已回到部署預設 ${dollars(defaultUSD)}，新建立的試跑就照這個上限。`
    : `已改為 ${dollars(usd)}，新建立的試跑就照這個上限。`;
}

export function RunModelBudget({
  setting,
  readable,
  refreshing,
}: {
  setting: Budget;
  readable: boolean;
  refreshing: boolean;
}) {
  const set = useRunModelBudget();
  const [value, setValue] = useState(setting.usd.toFixed(2));
  const usd = Number(value);
  const valid =
    value.trim() !== "" && Number.isFinite(usd) && usd >= setting.min_usd && usd <= setting.max_usd;
  const clearing = set.variables !== undefined && set.variables.usd === undefined;
  const expected = set.variables?.usd ?? setting.default_usd;
  const unverified = set.isSuccess && (!readable || setting.usd !== expected);
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-run-budget-heading"
      onInput={() => set.reset()}
    >
      <h2 id="admin-settings-run-budget-heading" tabIndex={-1}>
        每次試跑的模型花費上限
      </h2>
      {readable && !unverified && (
        <>
          <BudgetFacts setting={setting} />
          <details>
            <summary>調整花費上限</summary>
            <ActionForm
              id="admin-settings-run-budget"
              submitLabel="改上限"
              pending={set.isPending}
              error={clearing ? null : set.error}
              ready={valid}
              contextKey={value}
              onSubmit={(note) => valid && set.mutate({ usd, note })}
            >
              <div className="field">
                <label htmlFor="admin-settings-run-budget-value">
                  美元（{dollars(setting.min_usd)}–{dollars(setting.max_usd)}）
                </label>
                <input
                  id="admin-settings-run-budget-value"
                  type="number"
                  min={setting.min_usd}
                  max={setting.max_usd}
                  step={0.01}
                  value={value}
                  onChange={(event) => setValue(event.target.value)}
                  readOnly={set.isPending}
                />
              </div>
            </ActionForm>
            {setting.reason && (
              <ActionForm
                id="admin-settings-run-budget-clear"
                submitLabel={`回到部署預設 ${dollars(setting.default_usd)}`}
                pending={set.isPending}
                error={clearing ? set.error : null}
                onSubmit={(note) => set.mutate({ note })}
              />
            )}
          </details>
        </>
      )}
      {set.isSuccess && (
        <SettingsResult id="admin-settings-run-budget-result" verified={!unverified}>
          {unverified
            ? "花費上限變更請求已完成；目前設定尚未確認，請重新整理。"
            : resultSentence(set.variables.usd, setting.default_usd)}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-run-budget-result"
          onClick={() => {
            setValue(setting.usd.toFixed(2));
            set.reset();
            document.getElementById("admin-settings-run-budget-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的花費上限"}
        </button>
      )}
    </section>
  );
}
