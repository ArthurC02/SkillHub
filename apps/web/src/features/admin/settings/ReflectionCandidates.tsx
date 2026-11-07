import { useState } from "react";
import {
  SetReflectionCandidatesBodyCountMaximum as MAX_COUNT,
  SetReflectionCandidatesBodyRoundCreditCapMaximum as MAX_CAP,
} from "@skillhub/api-client-ts/limits";
import { useReflectionCandidatesSetting, type PlatformSettings } from "../admin.service";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";
import { SettingsResult } from "./SettingsResult";

type Setting = PlatformSettings["reflection_candidates"];
type ReflectionCandidatesProps = { setting: Setting; readable: boolean; refreshing: boolean };

function SettingFacts({ setting }: { setting: Setting }) {
  return (
    <>
      <p>
        每輪 {setting.count} 個候選；
        {setting.round_credit_cap === undefined
          ? "每輪點數上限尚未設定"
          : `每輪最多 ${setting.round_credit_cap} 點`}
      </p>
      <p className="note">
        一輪找改進時，每個候選各問一次模型，各自先看指示、Script
        與工具、參考資料與範例，第四個不限方向。
        整輪只在開始前檢查一次點數、結束後扣一次。上限尚未設定時，多於一個候選的一輪不會開始。
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

function isWhole(value: string, min: number, max: number) {
  const n = Number(value);
  return value.trim() !== "" && Number.isInteger(n) && n >= min && n <= max;
}

export function ReflectionCandidates({ setting, readable, refreshing }: ReflectionCandidatesProps) {
  const save = useReflectionCandidatesSetting();
  const [count, setCount] = useState(String(setting.count));
  const [cap, setCap] = useState(String(setting.round_credit_cap ?? ""));
  const capValid = cap.trim() === "" || isWhole(cap, 1, MAX_CAP);
  const valid = isWhole(count, 1, MAX_COUNT) && capValid;
  const unverified =
    save.isSuccess &&
    (!readable ||
      setting.count !== save.variables.count ||
      setting.round_credit_cap !== save.variables.round_credit_cap);
  return (
    <section
      className="admin-settings-policy"
      aria-labelledby="admin-settings-reflection-candidates-heading"
      onInput={() => save.reset()}
    >
      <h2 id="admin-settings-reflection-candidates-heading" tabIndex={-1}>
        找改進的候選數
      </h2>
      {readable && !unverified && (
        <>
          <SettingFacts setting={setting} />
          <details>
            <summary>調整候選數與上限</summary>
            <ActionForm
              id="admin-settings-reflection-candidates"
              submitLabel="儲存候選數與上限"
              pending={save.isPending}
              error={save.error}
              ready={valid}
              contextKey={`${count}:${cap}`}
              onSubmit={(note) =>
                valid &&
                save.mutate({
                  count: Number(count),
                  ...(cap.trim() === "" ? {} : { round_credit_cap: Number(cap) }),
                  note,
                })
              }
            >
              <div className="field">
                <label htmlFor="admin-settings-reflection-candidates-count">
                  每輪候選數（1–{MAX_COUNT}）
                </label>
                <input
                  id="admin-settings-reflection-candidates-count"
                  type="number"
                  min={1}
                  max={MAX_COUNT}
                  step={1}
                  value={count}
                  onChange={(event) => setCount(event.target.value)}
                  readOnly={save.isPending}
                />
              </div>
              <div className="field">
                <label htmlFor="admin-settings-reflection-candidates-cap">
                  每輪點數上限（留空＝不設定）
                </label>
                <input
                  id="admin-settings-reflection-candidates-cap"
                  type="number"
                  min={1}
                  max={MAX_CAP}
                  step={1}
                  value={cap}
                  onChange={(event) => setCap(event.target.value)}
                  readOnly={save.isPending}
                />
              </div>
            </ActionForm>
          </details>
        </>
      )}
      {save.isSuccess && (
        <SettingsResult id="admin-settings-reflection-candidates-result" verified={!unverified}>
          {unverified
            ? "候選數變更請求已完成；目前設定尚未確認，請重新整理。"
            : `已儲存，下一輪找改進會問 ${save.variables.count} 個候選。`}
        </SettingsResult>
      )}
      {unverified && readable && (
        <button
          type="button"
          disabled={refreshing}
          aria-describedby="admin-settings-reflection-candidates-result"
          onClick={() => {
            save.reset();
            document.getElementById("admin-settings-reflection-candidates-heading")?.focus();
          }}
        >
          {refreshing ? "讀取目前設定中…" : "檢視目前的候選數設定"}
        </button>
      )}
    </section>
  );
}
