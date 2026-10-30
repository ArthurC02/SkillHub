import { Link } from "@tanstack/react-router";
import {
  useContinuationRounds,
  useJudgePanelSwitch,
  usePlatformSettings,
  useRunSettings,
  type PlatformSettings,
  type RunSettings,
} from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";
import { useSettingDraft } from "../components/useSettingDraft";
import { SettingsResult } from "./SettingsResult";
import { RunModelBudget } from "./RunModelBudget";
import { RunTokensPerMinute } from "./RunTokensPerMinute";
import "./AdminSettings.page.css";

function JudgePanel({
  panel,
  readable,
  refreshing,
}: {
  panel: PlatformSettings["judge_panel"];
  readable: boolean;
  refreshing: boolean;
}) {
  const toggle = useJudgePanelSwitch();
  const next = !panel.enabled;
  const unverified = toggle.isSuccess && (!readable || panel.enabled !== toggle.variables.enabled);
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-judge-panel-heading"
      onInput={() => toggle.reset()}
    >
      <h2 id="admin-settings-judge-panel-heading" tabIndex={-1}>
        評估判定的評審團
      </h2>
      {readable && !unverified && (
        <>
          <p className="badge-row">
            <span className={panel.enabled ? "badge badge-danger" : "badge"}>
              {panel.enabled ? "開啟：三個判定逐條多數決" : "關閉：一個判定（預設）"}
            </span>
          </p>
          <p className="note">
            需要模型判定的驗收條件，開啟後由三個判定各自判一次，每條條件取過半數的結果；沒有過半就記為無法判定。
            模型判定的費用約為三倍，任一個判定失敗整次評估就失敗。下一次評估就會照新的設定。
          </p>
          {panel.reason && (
            <>
              <p>理由：{panel.reason}</p>
              {panel.set_at && (
                <p>
                  設定於 <Timestamp at={panel.set_at} />
                </p>
              )}
            </>
          )}
          <details>
            <summary>調整評審團</summary>
            <ActionForm
              id="admin-settings-judge-panel"
              submitLabel={next ? "開啟評審團" : "關閉評審團"}
              tone={next ? "caution" : undefined}
              pending={toggle.isPending}
              error={toggle.error}
              contextKey={String(next)}
              onSubmit={(note) => toggle.mutate({ enabled: next, note })}
            />
          </details>
        </>
      )}
      {toggle.isSuccess && (
        <SettingsResult id="admin-settings-judge-panel-result" verified={!unverified}>
          {unverified
            ? "評審團變更請求已完成；目前設定尚未確認，請重新整理。"
            : toggle.variables.enabled
              ? "已開啟，需要模型判定的條件由三個判定投票。"
              : "已關閉，下一次評估回到一個判定。"}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-judge-panel-result"
          onClick={() => {
            toggle.reset();
            document.getElementById("admin-settings-judge-panel-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的評審團設定"}
        </button>
      )}
    </section>
  );
}

function ContinuationRounds({
  setting,
  readable,
  refreshing,
}: {
  setting: RunSettings["continuation_rounds"];
  readable: boolean;
  refreshing: boolean;
}) {
  const set = useContinuationRounds();
  const draft = useSettingDraft(setting.rounds, set.isSuccess);
  const editableValue = draft.value;
  const rounds = Number(editableValue);
  const unverified = set.isSuccess && (!readable || setting.rounds !== set.variables.rounds);
  const valid =
    editableValue.trim() !== "" &&
    Number.isInteger(rounds) &&
    rounds >= setting.min &&
    rounds <= setting.max;
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-rounds-heading"
      onInput={() => set.reset()}
    >
      <h2 id="admin-settings-rounds-heading" tabIndex={-1}>
        一段對話可以回答幾輪
      </h2>
      {readable && !unverified && (
        <>
          <p>
            目前：<strong>{setting.rounds}</strong> 輪（可設 {setting.min}–{setting.max} 輪）
          </p>
          <p className="note">
            每回答一組問題並建立接續
            Run，算一輪。下一次回答就讀新的設定；已達上限的對話保留原有的試跑，但不能再加一輪。
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
          {draft.changed && (
            <p role="status">輪數已變更；草稿改為目前的 {setting.rounds} 輪，請確認後再送出。</p>
          )}
          <details>
            <summary>調整回答輪數上限</summary>
            <ActionForm
              id="admin-settings-rounds"
              submitLabel="改輪數"
              pending={set.isPending}
              error={set.error}
              ready={valid}
              contextKey={editableValue}
              done={set.isSuccess && "變更請求已完成。"}
              onSubmit={(note) => valid && set.mutate({ rounds, note })}
            >
              <div className="field">
                <label htmlFor="admin-settings-rounds-value">
                  輪數（{setting.min}–{setting.max}）
                </label>
                <input
                  id="admin-settings-rounds-value"
                  type="number"
                  min={setting.min}
                  max={setting.max}
                  step={1}
                  value={editableValue}
                  onChange={(event) => {
                    draft.update(event.target.value);
                    set.reset();
                  }}
                  readOnly={set.isPending}
                />
              </div>
            </ActionForm>
          </details>
        </>
      )}
      {set.isSuccess && (
        <SettingsResult id="admin-settings-rounds-result" verified={!unverified}>
          {unverified
            ? "輪數變更請求已完成；目前設定尚未確認，請重新整理。"
            : `已改為 ${set.variables.rounds} 輪，下一次回答就照這個值。`}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-rounds-result"
          onClick={() => {
            draft.update(String(setting.rounds));
            set.reset();
            document.getElementById("admin-settings-rounds-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的回答輪數"}
        </button>
      )}
    </section>
  );
}

export function AdminSettings() {
  const settings = usePlatformSettings();
  const runSettings = useRunSettings();
  return (
    <AdminPage
      heading="平台設定"
      lede="調整平台的評估方式、對話限制與試跑花費上限；每次改動都要附理由，並記進動作紀錄。"
    >
      <p className="note">
        每日花費上限與啟停，前往{" "}
        <Link to="/admin/agents" search={{}} hash="admin-agent-controls">
          平台 Agent
        </Link>{" "}
        調整。
      </p>
      <button
        type="button"
        disabled={settings.isFetching || runSettings.isFetching}
        onClick={() => void Promise.all([settings.refetch(), runSettings.refetch()])}
      >
        {settings.isFetching || runSettings.isFetching ? "重新整理中…" : "重新整理設定"}
      </button>
      <div className="admin-settings-grid">
        <div>
          {settings.isPending && <Loading what="平台設定" />}
          <ReadFailure error={settings.error} what="平台設定">
            <p role="alert">暫時無法讀取平台設定，請再試一次。</p>
            <button
              type="button"
              disabled={settings.isFetching}
              onClick={() => {
                void settings.refetch();
                (
                  document.getElementById("admin-settings-judge-panel-heading") ??
                  document.getElementById("main-content")
                )?.focus();
              }}
            >
              再讀取平台設定
            </button>
          </ReadFailure>
          {settings.data && (
            <JudgePanel
              panel={settings.data.judge_panel}
              readable={!settings.error}
              refreshing={settings.isFetching}
            />
          )}
        </div>
        <div>
          {runSettings.isPending && <Loading what="試跑設定" />}
          <ReadFailure error={runSettings.error} what="試跑設定">
            <p role="alert">暫時無法讀取試跑設定，請再試一次。</p>
            <button
              type="button"
              disabled={runSettings.isFetching}
              onClick={() => {
                void runSettings.refetch();
                (
                  document.getElementById("admin-settings-rounds-heading") ??
                  document.getElementById("main-content")
                )?.focus();
              }}
            >
              再讀取試跑設定
            </button>
          </ReadFailure>
          {runSettings.data && (
            <>
              <ContinuationRounds
                setting={runSettings.data.continuation_rounds}
                readable={!runSettings.error}
                refreshing={runSettings.isFetching}
              />
              <RunModelBudget
                setting={runSettings.data.model_budget}
                readable={!runSettings.error}
                refreshing={runSettings.isFetching}
              />
              <RunTokensPerMinute
                setting={runSettings.data.tokens_per_minute}
                readable={!runSettings.error}
                refreshing={runSettings.isFetching}
              />
            </>
          )}
        </div>
      </div>
    </AdminPage>
  );
}
