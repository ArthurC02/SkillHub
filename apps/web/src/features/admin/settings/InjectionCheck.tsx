import { useState } from "react";
import { SetInjectionCheckBodyCreditsPerMillionTokensMaximum as MAX_PRICE } from "@skillhub/api-client-ts/limits";
import { useInjectionCheckSwitch, type PlatformSettings } from "../admin.service";
import { points } from "../../../shared/format";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";
import { SettingsResult } from "./SettingsResult";
import { unverifiedSettingSentence } from "./SettingsResult.model";

type Check = PlatformSettings["injection_check"];
type InjectionCheckProps = { check: Check; readable: boolean; refreshing: boolean };

const MIN_PRICE = 1;

function CheckFacts({ check }: { check: Check }) {
  return (
    <>
      <p className="badge-row">
        <span className={check.enabled ? "badge badge-danger" : "badge"}>
          {check.enabled ? "開啟：判定前先檢查輸出" : "關閉（預設）"}
        </span>
      </p>
      <p>
        {check.credits_per_million_tokens === undefined
          ? "尚未定價"
          : `每百萬輸入 token ${points(check.credits_per_million_tokens)}`}
      </p>
      <p className="note">
        開啟後，需要模型判定的評估會另外問一次：試跑的輸出、產出檔與執行紀錄裡，有沒有對評審下指令的文字。結果顯示在判定旁邊，不改變判定。
        閘道把這類呼叫記成 0 元，平台照這裡的單價記成估算的成本，不向使用者扣點。
      </p>
      {check.reason && (
        <>
          <p>理由：{check.reason}</p>
          {check.set_at && (
            <p>
              設定於 <Timestamp at={check.set_at} />
            </p>
          )}
        </>
      )}
    </>
  );
}

export function InjectionCheck({ check, readable, refreshing }: InjectionCheckProps) {
  const toggle = useInjectionCheckSwitch();
  const [value, setValue] = useState(String(check.credits_per_million_tokens ?? ""));
  const price = Number(value);
  const valid =
    value.trim() !== "" && Number.isInteger(price) && price >= MIN_PRICE && price <= MAX_PRICE;
  const next = !check.enabled;
  const unverified = toggle.isSuccess && (!readable || check.enabled !== toggle.variables.enabled);
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-injection-check-heading"
      onInput={() => toggle.reset()}
    >
      <h2 id="admin-settings-injection-check-heading" tabIndex={-1}>
        判定前的注入檢查
      </h2>
      {readable && !unverified && (
        <>
          <CheckFacts check={check} />
          <details>
            <summary>調整注入檢查</summary>
            <ActionForm
              id="admin-settings-injection-check"
              submitLabel={next ? "開啟注入檢查" : "關閉注入檢查"}
              tone={next ? "caution" : undefined}
              pending={toggle.isPending}
              error={toggle.error}
              ready={valid}
              contextKey={`${next}:${value}`}
              onSubmit={(note) =>
                valid && toggle.mutate({ enabled: next, credits_per_million_tokens: price, note })
              }
            >
              <div className="field">
                <label htmlFor="admin-settings-injection-check-price">
                  每百萬輸入 token 的點數（{MIN_PRICE}–{MAX_PRICE}）
                </label>
                <input
                  id="admin-settings-injection-check-price"
                  type="number"
                  min={MIN_PRICE}
                  max={MAX_PRICE}
                  step={1}
                  value={value}
                  onChange={(event) => setValue(event.target.value)}
                  readOnly={toggle.isPending}
                />
              </div>
            </ActionForm>
          </details>
        </>
      )}
      {toggle.isSuccess && (
        <SettingsResult id="admin-settings-injection-check-result" verified={!unverified}>
          {unverified
            ? unverifiedSettingSentence("注入檢查")
            : toggle.variables.enabled
              ? "已開啟，下一次需要模型判定的評估會先檢查輸出。"
              : "已關閉，下一次評估不再檢查輸出。"}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-injection-check-result"
          onClick={() => {
            toggle.reset();
            document.getElementById("admin-settings-injection-check-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的注入檢查設定"}
        </button>
      )}
    </section>
  );
}
