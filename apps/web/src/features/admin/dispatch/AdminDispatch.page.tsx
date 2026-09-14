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
      {status.data && (
        <>
          <p>
            <span className={status.data.dispatching ? "badge" : "badge badge-danger"}>
              {status.data.dispatching ? "正在派送" : "停止派送"}
            </span>
          </p>
          {status.data.halts.length === 0 ? (
            <p>煞車：0 個。</p>
          ) : (
            <ul className="download-list">
              {status.data.halts.map((halt) => (
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

      <h2>宣告或解除</h2>
      <div className="field">
        <label htmlFor="admin-halt-provider">節點名稱（留空是整個叢集）</label>
        <input
          id="admin-halt-provider"
          value={provider}
          onChange={(event) => setProvider(event.target.value)}
        />
      </div>
      <h3>停止派送</h3>
      <ActionForm
        id="admin-halt-declare"
        submitLabel="停止派送"
        pending={declare.isPending}
        error={declare.error}
        done={declare.data?.note}
        onSubmit={(note) => declare.mutate({ note, provider: target })}
      />
      <h3>恢復派送</h3>
      <ActionForm
        id="admin-halt-lift"
        submitLabel="恢復派送"
        pending={lift.isPending}
        error={lift.error}
        done={lift.isSuccess && "已解除，上面的狀態已更新。"}
        onSubmit={(note) => lift.mutate({ note, provider: target })}
      />
    </AdminPage>
  );
}
