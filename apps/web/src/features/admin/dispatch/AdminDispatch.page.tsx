import { useState } from "react";
import { useDispatchHalt, useDispatchStatus } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";

const HALT_SOURCE: Record<string, string> = {
  p1_incident: "P1 事故：只有人能解除",
  orphan_threshold: "孤兒門檻：連續兩輪低於門檻會自動解除",
};

function recoveryBlockReason(
  status: ReturnType<typeof useDispatchStatus>,
  selected: boolean,
  declaring: boolean,
) {
  if (declaring) return "正在停止派送，完成後才能恢復。";
  if (!status.isSuccess || status.isFetching) {
    return "必須先讀到目前的派送狀態，才能恢復派送。";
  }
  if (status.data.halts.length === 0) return "目前沒有煞車可解除。";
  return selected ? undefined : "先選擇目前清單中的煞車，才能恢復派送。";
}

function visibleStatus(status: ReturnType<typeof useDispatchStatus>) {
  return status.isSuccess && !status.isFetching ? status.data : undefined;
}

export function AdminDispatch() {
  const status = useDispatchStatus();
  const declare = useDispatchHalt("PUT");
  const lift = useDispatchHalt("DELETE");
  const [provider, setProvider] = useState("");
  const [recoveryTarget, setRecoveryTarget] = useState("");
  const target = provider.trim() || undefined;
  const changing = declare.isPending || lift.isPending;
  const currentStatus = visibleStatus(status);
  const selectedHalt = currentStatus?.halts.find((halt) => halt.target === recoveryTarget);
  const recoveryBlock = recoveryBlockReason(status, selectedHalt !== undefined, declare.isPending);

  return (
    <AdminPage heading="派送煞車">
      {status.isFetching && <Loading what="派送狀態" />}
      <ReadFailure error={status.error} what="派送狀態" />
      {currentStatus && (
        <>
          <p>
            <span className={currentStatus.dispatching ? "badge" : "badge badge-danger"}>
              {currentStatus.dispatching ? "正在派送" : "停止派送"}
            </span>
          </p>
          {currentStatus.halts.length === 0 ? (
            <p>煞車：0 個。</p>
          ) : (
            <ul className="download-list">
              {currentStatus.halts.map((halt) => (
                <li className="download-item" key={`${halt.target}-${halt.source}`}>
                  <p>
                    <strong>{halt.target === "pool" ? "整個叢集" : `節點 ${halt.target}`}</strong>
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
        </>
      )}

      <h2>停止派送</h2>
      <ActionForm
        id="admin-halt-declare"
        submitLabel="停止派送"
        pending={declare.isPending}
        error={declare.error}
        done={declare.data?.note}
        contextKey={target ?? "pool"}
        tone="caution"
        ready={!lift.isPending}
        unavailableReason={lift.isPending ? "正在恢復派送，完成後才能再次停止。" : undefined}
        onSubmit={(note) => {
          lift.reset();
          declare.mutate({ note, provider: target });
        }}
      >
        <div className="field">
          <label htmlFor="admin-halt-provider">節點名稱（留空是整個叢集）</label>
          <input
            id="admin-halt-provider"
            value={provider}
            onChange={(event) => {
              setProvider(event.target.value);
              declare.reset();
            }}
            readOnly={changing}
          />
        </div>
        <p className="note">本次停止範圍：{target ? `節點 ${target}` : "整個叢集"}。</p>
      </ActionForm>
      <h2>恢復派送</h2>
      <ActionForm
        id="admin-halt-lift"
        submitLabel="恢復派送"
        pending={lift.isPending}
        error={lift.error}
        done={lift.isSuccess && "解除請求已處理；若狀態讀取失敗，請重新整理確認。"}
        contextKey={recoveryTarget}
        ready={!recoveryBlock}
        unavailableReason={recoveryBlock}
        onSubmit={(note) => {
          if (!selectedHalt) return;
          declare.reset();
          lift.mutate({
            note,
            provider: selectedHalt.target === "pool" ? undefined : selectedHalt.target,
          });
        }}
      >
        <div className="field">
          <label htmlFor="admin-halt-recovery-target">要解除的煞車</label>
          <select
            id="admin-halt-recovery-target"
            value={recoveryTarget}
            onChange={(event) => {
              setRecoveryTarget(event.target.value);
              lift.reset();
            }}
            disabled={!status.isSuccess || status.isFetching || changing}
          >
            <option value="">請選擇目前的煞車</option>
            {currentStatus?.halts.map((halt) => (
              <option key={halt.target} value={halt.target}>
                {halt.target === "pool" ? "整個叢集" : `節點 ${halt.target}`} ·{" "}
                {HALT_SOURCE[halt.source] ?? halt.source}
              </option>
            ))}
          </select>
        </div>
        {selectedHalt && <p className="note">將解除的煞車原因：{selectedHalt.reason}</p>}
      </ActionForm>
    </AdminPage>
  );
}
