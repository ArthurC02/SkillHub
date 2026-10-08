import { useState } from "react";
import { dispatchState } from "../admin.model";
import {
  useDispatchHalt,
  useDispatchStatus,
  type DispatchHalt,
  type DispatchStatus,
} from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";

const HALT_SOURCE: Record<string, string> = {
  p1_incident: "P1 事故：只有人能解除",
  orphan_threshold: "孤兒門檻：連續兩輪低於門檻會自動解除",
};

function haltTargetLabel(target: string) {
  return target === "pool" ? "整個叢集" : "節點 " + target;
}

function DispatchOverview({ status }: { status: DispatchStatus }) {
  const state = dispatchState(status);
  return (
    <>
      <section aria-labelledby="admin-dispatch-state">
        <h2 id="admin-dispatch-state">目前派送</h2>
        <p role="status">
          <span
            className={
              "badge " +
              (state === "stopped"
                ? "badge-danger"
                : state === "partial"
                  ? "badge-warning"
                  : "badge-positive")
            }
          >
            {state === "stopped"
              ? "停止派送"
              : state === "partial"
                ? "部分節點停止派送"
                : "正在派送"}
          </span>
        </p>
        <p className="note">
          {state === "stopped"
            ? "目前沒有節點可接新的 Run。"
            : state === "partial"
              ? "其他節點仍可派送；下列煞車仍在生效。"
              : "平台可派送新的 Run。"}
        </p>
      </section>
      <section aria-labelledby="admin-dispatch-halts">
        <h2 id="admin-dispatch-halts">生效中的煞車</h2>
        {status.halts.length === 0 ? (
          <p>
            {state === "stopped" ? "未列出煞車，請確認節點設定與平台狀態。" : "沒有生效中的煞車。"}
          </p>
        ) : (
          <ul className="download-list">
            {status.halts.map((halt) => (
              <li className="download-item" key={halt.target + "-" + halt.source}>
                <p>
                  <strong>{haltTargetLabel(halt.target)}</strong>
                </p>
                <p className="badge-row">
                  <span className="badge">{HALT_SOURCE[halt.source] ?? halt.source}</span>
                </p>
                <p>理由：{halt.reason}</p>
                <p>
                  宣告於 <Timestamp at={halt.declared_at} />
                </p>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}

function LiftHalt({
  halts,
  lift,
}: {
  halts: DispatchHalt[];
  lift: ReturnType<typeof useDispatchHalt>;
}) {
  const [liftTarget, setLiftTarget] = useState("");
  const selectedHalt = halts.find((halt) => halt.target === liftTarget);
  return (
    <section aria-labelledby="admin-dispatch-lift">
      <h2 id="admin-dispatch-lift">解除煞車</h2>
      {halts.length === 0 ? (
        <p>目前沒有可解除的煞車。</p>
      ) : (
        <>
          <p className="note">
            解除前先重新整理狀態，並確認：觸發條件已消失、證據已保存、處置已記錄。
          </p>
          <div className="field">
            <label htmlFor="admin-halt-lift-target">要解除的對象</label>
            <select
              id="admin-halt-lift-target"
              value={selectedHalt?.target ?? ""}
              onChange={(event) => {
                setLiftTarget(event.target.value);
                lift.reset();
              }}
              disabled={lift.isPending}
            >
              <option value="">請選擇生效中的煞車</option>
              {halts.map((halt) => (
                <option value={halt.target} key={halt.target}>
                  {haltTargetLabel(halt.target)} · {HALT_SOURCE[halt.source] ?? halt.source}
                </option>
              ))}
            </select>
          </div>
          <ActionForm
            key={
              selectedHalt
                ? [
                    selectedHalt.target,
                    selectedHalt.source,
                    selectedHalt.reason,
                    selectedHalt.declared_at,
                  ].join("|")
                : ""
            }
            id="admin-halt-lift"
            submitLabel="恢復派送"
            pending={lift.isPending}
            error={lift.error}
            contextKey={selectedHalt?.target ?? ""}
            ready={Boolean(selectedHalt)}
            confirmationScope={
              selectedHalt &&
              "將解除" +
                haltTargetLabel(selectedHalt.target) +
                "的煞車；符合條件的 Run 會再次派送，其他煞車不受影響。"
            }
            confirmationLabel="確認恢復派送"
            onSubmit={(note) => {
              if (selectedHalt) {
                lift.mutate({
                  note,
                  provider: selectedHalt.target === "pool" ? undefined : selectedHalt.target,
                });
              }
            }}
          />
        </>
      )}
      {lift.isSuccess && (
        <p className="notice notice-success" role="status">
          解除請求已完成，請核對上方派送狀態。
        </p>
      )}
    </section>
  );
}

export function AdminDispatch() {
  const status = useDispatchStatus();
  const declare = useDispatchHalt("PUT");
  const lift = useDispatchHalt("DELETE");
  const [provider, setProvider] = useState("");
  const target = provider.trim() || undefined;

  return (
    <AdminPage heading="派送煞車">
      {status.isPending && <Loading what="派送狀態" />}
      <ReadFailure error={status.error} what="派送狀態" />
      {status.data && !status.error && (
        <p className="note">
          派送狀態上次取得於{" "}
          <Timestamp at={new Date(status.dataUpdatedAt).toISOString()} relative />。{" "}
          <button type="button" disabled={status.isFetching} onClick={() => void status.refetch()}>
            {status.isFetching ? "重新整理中…" : "重新整理派送狀態"}
          </button>
        </p>
      )}
      {status.data && !status.error && <DispatchOverview status={status.data} />}

      <section aria-labelledby="admin-dispatch-declare">
        <h2 id="admin-dispatch-declare">宣告 P1 煞車</h2>
        <div className="field">
          <label htmlFor="admin-halt-provider">要停止的節點名稱（留空是整個叢集）</label>
          <input
            id="admin-halt-provider"
            value={provider}
            onChange={(event) => {
              setProvider(event.target.value);
              declare.reset();
            }}
            readOnly={declare.isPending || lift.isPending}
          />
        </div>
        <ActionForm
          id="admin-halt-declare"
          submitLabel="停止派送"
          pending={declare.isPending}
          error={declare.error}
          done={declare.data?.note}
          contextKey={target ?? "pool"}
          tone="caution"
          onSubmit={(note) => declare.mutate({ note, provider: target })}
        />
      </section>

      {status.data && !status.error && <LiftHalt halts={status.data.halts} lift={lift} />}
    </AdminPage>
  );
}
