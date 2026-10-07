import { useState } from "react";
import { ApiError } from "../../../core/api/client";
import { useDispatchHalt, useDispatchStatus, type DispatchStatus } from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ActionForm } from "../components/ActionForm";
import { OPERATOR_NOTE_MAX_BYTES } from "../admin.model";

const HALT_SOURCE: Record<string, string> = {
  p1_incident: "P1 事故：只有人能解除",
  orphan_threshold: "孤兒門檻：連續兩輪低於門檻會自動解除",
};
const HALT_DONE =
  "新的 Run 已停止派往本次範圍，清理與孤兒資源拆除也已暫停。煞車不會自動解除；若狀態讀取失敗，請重新整理確認。";

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

function StatusFeedback({ status }: { status: ReturnType<typeof useDispatchStatus> }) {
  return (
    <>
      {status.isPending && status.isFetching && <Loading what="派送狀態" />}
      {status.data && status.isFetching && (
        <p className="note">正在確認最新派送狀態；確認期間不可解除煞車。</p>
      )}
      <ReadFailure
        error={status.error}
        what="派送狀態"
        onRetry={() => void status.refetch()}
        retrying={status.isFetching}
      />
    </>
  );
}

function P1RecoveryChecklist({ source }: { source?: string }) {
  if (source !== "p1_incident") return null;
  return (
    <p className="notice notice-warning" id="admin-p1-recovery-checklist">
      解除 P1 前：確認觸發條件已排除、現場與處置證據已保存，且 sev/P1
      事件單已建立並記下自動動作結果。解除只恢復派送，不會補做暫停的清理。
    </p>
  );
}

function DispatchOverview({ status }: { status: DispatchStatus }) {
  return (
    <>
      <p>
        <span className={status.dispatching ? "badge" : "badge badge-danger"}>
          {status.dispatching ? "正在派送" : "停止派送"}
        </span>
      </p>
      {status.halts.length === 0 ? (
        <p>煞車：0 個。</p>
      ) : (
        <ul className="download-list">
          {status.halts.map((halt) => (
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
  );
}

export function AdminDispatch() {
  const status = useDispatchStatus();
  const declare = useDispatchHalt("PUT");
  const lift = useDispatchHalt("DELETE");
  const [provider, setProvider] = useState("");
  const [recoveryTarget, setRecoveryTarget] = useState("");
  const [recoveryFormRevision, setRecoveryFormRevision] = useState(0);
  const target = provider.trim() || undefined;
  const changing = declare.isPending || lift.isPending;
  const currentStatus = visibleStatus(status);
  const selectedHalt = currentStatus?.halts.find((halt) => halt.target === recoveryTarget);
  const recoveryBlock = recoveryBlockReason(status, selectedHalt !== undefined, declare.isPending);

  return (
    <AdminPage heading="派送煞車">
      <StatusFeedback status={status} />
      {currentStatus && <DispatchOverview status={currentStatus} />}

      <h2>停止派送</h2>
      <ActionForm
        id="admin-halt-declare"
        submitLabel="停止派送"
        pending={declare.isPending}
        error={declare.error}
        maxNoteBytes={OPERATOR_NOTE_MAX_BYTES}
        done={declare.isSuccess && HALT_DONE}
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
      {lift.error instanceof ApiError && lift.error.status === 409 && (
        <p className="notice notice-warning" role="alert">
          煞車在你查看後已變更，沒有解除任何煞車。請等最新狀態載入，再重新選擇並確認事故。
        </p>
      )}
      <ActionForm
        key={recoveryFormRevision}
        id="admin-halt-lift"
        submitLabel="恢復派送"
        pending={lift.isPending}
        error={lift.error}
        maxNoteBytes={OPERATOR_NOTE_MAX_BYTES}
        done={lift.isSuccess && "解除請求已處理；若狀態讀取失敗，請重新整理確認。"}
        contextKey={recoveryTarget}
        ready={!recoveryBlock}
        unavailableReason={recoveryBlock}
        onSubmit={(note) => {
          if (!selectedHalt) return;
          declare.reset();
          lift.mutate(
            {
              note,
              provider: selectedHalt.target === "pool" ? undefined : selectedHalt.target,
              halt_id: selectedHalt.halt_id,
              generation: selectedHalt.generation,
            },
            {
              onError: (error) => {
                if (error instanceof ApiError && error.status === 409) {
                  setRecoveryTarget("");
                  setRecoveryFormRevision((revision) => revision + 1);
                }
              },
            },
          );
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
        <P1RecoveryChecklist source={selectedHalt?.source} />
      </ActionForm>
    </AdminPage>
  );
}
