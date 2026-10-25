import { useRunModelBudget, type RunSettings } from "../admin.service";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";
import { SettingsResult } from "./SettingsResult";
import { useSettingDraft } from "../components/useSettingDraft";

type Budget = RunSettings["model_budget"];
type RunModelBudgetProps = { setting: Budget; readable: boolean; refreshing: boolean };

const points = (credits: number) => `${credits} 點`;

function BudgetFacts({ setting }: { setting: Budget }) {
  return (
    <>
      <p>
        目前：每次試跑最多花 <strong>{points(setting.credits)}</strong>的模型費用
        {setting.reason ? `（部署預設 ${points(setting.default_credits)}）` : "（部署預設）"}
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

function resultSentence(credits: number | undefined, defaultCredits: number) {
  return credits === undefined
    ? `已回到部署預設 ${points(defaultCredits)}，新建立的試跑就照這個上限。`
    : `已改為 ${points(credits)}，新建立的試跑就照這個上限。`;
}

export function RunModelBudget({ setting, readable, refreshing }: RunModelBudgetProps) {
  const set = useRunModelBudget();
  const draft = useSettingDraft(setting.credits, set.isSuccess);
  const value = draft.value;
  const credits = Number(value);
  const valid =
    value.trim() !== "" &&
    Number.isInteger(credits) &&
    credits >= setting.min_credits &&
    credits <= setting.max_credits;
  const clearing = set.variables !== undefined && set.variables.credits === undefined;
  const expected = set.variables?.credits ?? setting.default_credits;
  const unverified = set.isSuccess && (!readable || setting.credits !== expected);
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
          {draft.changed && (
            <p role="status">
              設定已變更；草稿改為目前的 {points(setting.credits)}，請確認後再送出。
            </p>
          )}
          <details>
            <summary>調整花費上限</summary>
            <ActionForm
              id="admin-settings-run-budget"
              submitLabel="改上限"
              pending={set.isPending}
              error={clearing ? null : set.error}
              ready={valid}
              contextKey={value}
              onSubmit={(note) => valid && set.mutate({ credits, note })}
            >
              <div className="field">
                <label htmlFor="admin-settings-run-budget-value">
                  點數（{setting.min_credits}–{setting.max_credits}）
                </label>
                <input
                  id="admin-settings-run-budget-value"
                  type="number"
                  min={setting.min_credits}
                  max={setting.max_credits}
                  step={1}
                  value={value}
                  onChange={(event) => draft.update(event.target.value)}
                  readOnly={set.isPending}
                />
              </div>
            </ActionForm>
            {setting.reason && (
              <ActionForm
                id="admin-settings-run-budget-clear"
                submitLabel={`回到部署預設 ${points(setting.default_credits)}`}
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
            : resultSentence(set.variables.credits, setting.default_credits)}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-run-budget-result"
          onClick={() => {
            draft.update(String(setting.credits));
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
