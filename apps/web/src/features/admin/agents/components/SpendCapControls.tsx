import { useEffect, useRef, useState } from "react";
import {
  MAX_AGENT_SPEND_CAP_MICROS,
  usd,
  type usePlatformAgentSpendCap,
  type PlatformAgent,
} from "../../admin.service";
import { ActionForm } from "../../components/ActionForm";

function dollarsToMicros(text: string): number | null {
  if (!/^\d+(\.\d{1,6})?$/.test(text.trim())) return null;
  const micros = Math.round(Number(text) * 1_000_000);
  return micros > 0 && micros <= MAX_AGENT_SPEND_CAP_MICROS ? micros : null;
}

export function SpendCapControls({
  agent,
  readable,
  change,
  onCompleted,
}: {
  agent: PlatformAgent;
  readable: boolean;
  change: ReturnType<typeof usePlatformAgentSpendCap>;
  onCompleted: (current: PlatformAgent) => void;
}) {
  const [dollars, setDollars] = useState(String(agent.daily_spend_cap_usd_micros / 1_000_000));
  const [sourceMicros, setSourceMicros] = useState(agent.daily_spend_cap_usd_micros);
  const staleDraft = sourceMicros !== agent.daily_spend_cap_usd_micros;
  const editableDollars = staleDraft
    ? String(agent.daily_spend_cap_usd_micros / 1_000_000)
    : dollars;
  const micros = dollarsToMicros(editableDollars);
  const restoring = change.variables?.daily_spend_cap_usd_micros === null;
  const setting = change.isPending && !restoring;
  const clearing = change.isPending && restoring;
  const readOnly = change.isPending || !readable;
  const busyReason = change.isPending ? "此上限正在處理變更，完成後再操作。" : undefined;
  const fieldId = `admin-agent-${agent.name}-cap`;
  const send = (value: number | null, note: string) =>
    change.mutate(
      { name: agent.name, daily_spend_cap_usd_micros: value, note },
      {
        onSuccess: (current) => {
          setDollars(String(current.daily_spend_cap_usd_micros / 1_000_000));
          setSourceMicros(current.daily_spend_cap_usd_micros);
          onCompleted(current);
        },
      },
    );
  return (
    <div hidden={!readable} onInput={() => change.reset()}>
      <details>
        <summary>調整每日花費上限</summary>
        {staleDraft && <p role="status">上限已變更；草稿改為目前值，請確認後再送出。</p>}
        <ActionForm
          id={`${fieldId}-set`}
          submitLabel={`改 ${agent.name} 的每日上限`}
          pending={setting}
          error={!restoring ? change.error : undefined}
          ready={readable && micros !== null && !clearing}
          readOnlyNote={readOnly}
          blockedReason={busyReason}
          contextKey={String(micros)}
          done={
            change.isSuccess &&
            change.variables.daily_spend_cap_usd_micros !== null &&
            "變更請求已完成。"
          }
          onSubmit={(note) => micros !== null && send(micros, note)}
        >
          <div className="field">
            <label htmlFor={fieldId}>
              每日花費上限（美元，最多 {usd(MAX_AGENT_SPEND_CAP_MICROS)}）
            </label>
            <input
              id={fieldId}
              inputMode="decimal"
              value={editableDollars}
              onChange={(event) => {
                setDollars(event.target.value);
                setSourceMicros(agent.daily_spend_cap_usd_micros);
              }}
              readOnly={readOnly}
              aria-describedby={micros === null ? `${fieldId}-range` : undefined}
            />
            {micros === null && (
              <p id={`${fieldId}-range`} className="note">
                要填大於 0、最多 {usd(MAX_AGENT_SPEND_CAP_MICROS)} 的金額，小數最多六位。
              </p>
            )}
          </div>
        </ActionForm>
      </details>
      {agent.daily_spend_cap_overridden && (
        <details>
          <summary>恢復預設上限（{usd(agent.default_daily_spend_cap_usd_micros)}）</summary>
          <ActionForm
            id={`${fieldId}-clear`}
            submitLabel={`把 ${agent.name} 的上限改回預設 ${usd(agent.default_daily_spend_cap_usd_micros)}`}
            pending={clearing}
            ready={readable && !setting}
            readOnlyNote={readOnly}
            blockedReason={busyReason}
            error={restoring ? change.error : undefined}
            done={
              change.isSuccess &&
              change.variables.daily_spend_cap_usd_micros === null &&
              "恢復請求已完成。"
            }
            onSubmit={(note) => send(null, note)}
          />
        </details>
      )}
    </div>
  );
}

export function SpendCapResult({
  completed,
  unverified,
  fetching,
  refresh,
}: {
  completed: PlatformAgent;
  unverified: boolean;
  fetching: boolean;
  refresh: () => void;
}) {
  const result = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    result.current?.focus({ preventScroll: true });
    result.current?.scrollIntoView?.({ block: "center", behavior: "instant" });
  }, [completed]);
  useEffect(() => {
    if (document.activeElement === result.current) {
      result.current?.scrollIntoView?.({ block: "center", behavior: "instant" });
    }
  }, [unverified]);
  return (
    <>
      <p
        id={`admin-agent-${completed.name}-cap-result`}
        ref={result}
        tabIndex={-1}
        className={`notice ${unverified ? "notice-warning" : "notice-success"}`}
        role="status"
        aria-busy={fetching}
      >
        {completed.name} 的每日花費上限的變更請求已完成；
        {unverified
          ? "最新上限尚未確認。"
          : `目前顯示${completed.daily_spend_cap_overridden ? "營運者設定" : "預設"} ${usd(completed.daily_spend_cap_usd_micros)}；下一次執行採用這個上限，進行中的執行不變。`}
      </p>
      {unverified && (
        <button type="button" disabled={fetching} onClick={refresh}>
          {fetching ? "重新整理中…" : "重新整理此 Agent 的上限"}
        </button>
      )}
    </>
  );
}
