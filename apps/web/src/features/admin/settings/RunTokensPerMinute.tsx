import { useRunTokensPerMinute, type RunSettings } from "../admin.service";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";
import { SettingsResult } from "./SettingsResult";
import { useSettingDraft } from "../components/useSettingDraft";

type Rate = RunSettings["tokens_per_minute"];
type RunTokensPerMinuteProps = { setting: Rate; readable: boolean; refreshing: boolean };

const perMinute = (limit: number) => `每分鐘 ${limit.toLocaleString("en-US")} tokens`;

function RateFacts({ setting }: { setting: Rate }) {
  return (
    <>
      <p>
        目前：試跑的模型金鑰最多 <strong>{perMinute(setting.limit)}</strong>
        {setting.reason ? `（部署預設${perMinute(setting.default_limit)}）` : "（部署預設）"}
      </p>
      <p className="note">
        每個試跑嘗試拿到的模型金鑰，每分鐘最多用這麼多 tokens；超過時模型閘道會讓它稍後再試。
        之後發出的每一把金鑰都照新的值，包括已建立試跑的下一次嘗試。
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

function resultSentence(limit: number | undefined, defaultLimit: number) {
  return limit === undefined
    ? `已回到部署預設${perMinute(defaultLimit)}，之後發出的金鑰就照這個值。`
    : `已改為${perMinute(limit)}，之後發出的金鑰就照這個值。`;
}

export function RunTokensPerMinute({ setting, readable, refreshing }: RunTokensPerMinuteProps) {
  const set = useRunTokensPerMinute();
  const draft = useSettingDraft(setting.limit, set.isSuccess);
  const value = draft.value;
  const limit = Number(value);
  const valid =
    value.trim() !== "" && Number.isInteger(limit) && limit >= setting.min && limit <= setting.max;
  const clearing = set.variables !== undefined && set.variables.limit === undefined;
  const expected = set.variables?.limit ?? setting.default_limit;
  const unverified = set.isSuccess && (!readable || setting.limit !== expected);
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-run-tpm-heading"
      onInput={() => set.reset()}
    >
      <h2 id="admin-settings-run-tpm-heading" tabIndex={-1}>
        試跑模型金鑰的每分鐘 tokens 上限
      </h2>
      {readable && !unverified && (
        <>
          <RateFacts setting={setting} />
          {draft.changed && (
            <p role="status">設定已變更；草稿改為{perMinute(setting.limit)}，請確認後再送出。</p>
          )}
          <details>
            <summary>調整每分鐘 tokens 上限</summary>
            <ActionForm
              id="admin-settings-run-tpm"
              submitLabel="改每分鐘上限"
              pending={set.isPending}
              error={clearing ? null : set.error}
              ready={valid}
              contextKey={value}
              onSubmit={(note) => valid && set.mutate({ limit, note })}
            >
              <div className="field">
                <label htmlFor="admin-settings-run-tpm-value">
                  每分鐘 tokens（{setting.min}–{setting.max}）
                </label>
                <input
                  id="admin-settings-run-tpm-value"
                  type="number"
                  min={setting.min}
                  max={setting.max}
                  step={1}
                  value={value}
                  onChange={(event) => draft.update(event.target.value)}
                  readOnly={set.isPending}
                />
              </div>
            </ActionForm>
            {setting.reason && (
              <ActionForm
                id="admin-settings-run-tpm-clear"
                submitLabel={`回到部署預設${perMinute(setting.default_limit)}`}
                pending={set.isPending}
                error={clearing ? set.error : null}
                onSubmit={(note) => set.mutate({ note })}
              />
            )}
          </details>
        </>
      )}
      {set.isSuccess && (
        <SettingsResult id="admin-settings-run-tpm-result" verified={!unverified}>
          {unverified
            ? "每分鐘 tokens 上限變更請求已完成；目前設定尚未確認，請重新整理。"
            : resultSentence(set.variables.limit, setting.default_limit)}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-run-tpm-result"
          onClick={() => {
            draft.update(String(setting.limit));
            set.reset();
            document.getElementById("admin-settings-run-tpm-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的每分鐘 tokens 上限"}
        </button>
      )}
    </section>
  );
}
