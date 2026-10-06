import { useState } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import {
  useOperatorAuditLog,
  validAuditWorkspace,
  type OperatorAuditEvent,
} from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { AdminPage } from "../components/AdminPage";
import { ACTION_LABEL } from "../admin.model";
import { MetadataCell } from "./components/MetadataCell";

const RESOURCE_LABEL: Record<string, string> = {
  account: "帳號",
  credit_account: "點數帳戶",
  credit_entry: "點數分錄",
  dispatch: "派送",
  skill: "小工具",
  publication: "發佈物",
  model_budget: "模型呼叫逾時",
};

export function AdminAuditLog() {
  const { workspace_id } = useSearch({ from: "/admin/audit-log" });
  return (
    <AuditLog
      key={workspace_id === undefined ? "all" : `filter:${workspace_id}`}
      workspaceID={workspace_id ?? ""}
      emptyAddress={workspace_id === ""}
    />
  );
}

function AuditLog({ workspaceID, emptyAddress }: { workspaceID: string; emptyAddress: boolean }) {
  const navigate = useNavigate();
  const [draft, setDraft] = useState(workspaceID);
  const selected = draft.trim();
  const valid = validAuditWorkspace(selected);
  const queryMatches = selected === workspaceID;
  const addressError = emptyAddress && queryMatches;
  const log = useOperatorAuditLog(workspaceID, emptyAddress);

  return (
    <AdminPage heading="動作紀錄">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!valid) return;
          if (selected === workspaceID && !addressError) void log.refetch();
          else
            void navigate({
              to: "/admin/audit-log",
              search: { workspace_id: selected || undefined },
            });
        }}
      >
        <div className="field">
          <label htmlFor="admin-audit-workspace">Workspace ID（留空顯示全平台）</label>
          <input
            id="admin-audit-workspace"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            aria-invalid={!valid || addressError}
            aria-describedby={!valid || addressError ? "admin-audit-workspace-error" : undefined}
          />
        </div>
        <button type="submit" className="action" disabled={!valid}>
          {addressError ? "顯示全平台" : "套用"}
        </button>
      </form>
      {!valid && (
        <p id="admin-audit-workspace-error" role="alert">
          Workspace ID 格式不正確；請輸入完整 UUID，或清空欄位。
        </p>
      )}
      {addressError && (
        <p id="admin-audit-workspace-error" role="alert">
          網址中的 Workspace ID 為空；按「顯示全平台」移除錯誤條件。
        </p>
      )}
      {valid && !queryMatches && <p role="status">篩選條件已變更；按「套用」顯示新紀錄。</p>}
      {valid && queryMatches && workspaceID && (
        <p className="note">只列屬於這個 Workspace 的動作；派送等全平台動作不在篩選結果內。</p>
      )}
      <AuditLogResults
        workspaceID={workspaceID}
        log={log}
        visible={valid && queryMatches && !addressError}
      />
    </AdminPage>
  );
}

function AuditLogResults({
  workspaceID,
  log,
  visible,
}: {
  workspaceID: string;
  log: ReturnType<typeof useOperatorAuditLog>;
  visible: boolean;
}) {
  if (!visible) return null;
  const rows = log.data?.pages.flatMap((page) => page.events) ?? [];
  const showRows = log.data && !log.isRefetchError && !log.isRefetching;

  return (
    <>
      {(log.isPending || log.isRefetching) && <Loading what="動作紀錄" />}
      {!log.isRefetching && (
        <ReadFailure
          error={log.isFetchNextPageError ? undefined : log.error}
          what="動作紀錄"
          onRetry={() => void log.refetch()}
          retrying={log.isFetching}
        />
      )}
      {log.isFetchNextPageError && (
        <p role="alert">
          後續紀錄暫時無法讀取；目前只顯示已載入的 {rows.length} 筆，清單不完整。可以重試載入更多。
        </p>
      )}
      {showRows &&
        (rows.length === 0 ? (
          <p>{workspaceID ? "此 Workspace 的 operator 動作" : "operator 動作"}：0 筆。</p>
        ) : (
          <AuditEventsTable rows={rows} workspaceID={workspaceID} />
        ))}
      {showRows && log.hasNextPage && (
        <button type="button" disabled={log.isFetchingNextPage} onClick={() => log.fetchNextPage()}>
          {log.isFetchingNextPage
            ? "載入中…"
            : log.isFetchNextPageError
              ? "重試載入更多"
              : "載入更多"}
        </button>
      )}
    </>
  );
}

function AuditEventsTable({
  rows,
  workspaceID,
}: {
  rows: OperatorAuditEvent[];
  workspaceID: string;
}) {
  return (
    <div className="table-scroll">
      <table className="responsive-table">
        <caption>
          operator 動作{workspaceID ? `，Workspace ${workspaceID}` : ""}，新的在上面
        </caption>
        <thead>
          <tr>
            <th scope="col">時間</th>
            <th scope="col">動作</th>
            <th scope="col">operator</th>
            <th scope="col">對象</th>
            <th scope="col">Workspace</th>
            <th scope="col">內容</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((event, index) => (
            <tr key={`${event.action}-${index}`}>
              <td data-label="時間">
                <Timestamp at={event.occurred_at} />
              </td>
              <th scope="row" data-label="動作">
                {ACTION_LABEL[event.action] ?? event.action}
              </th>
              <td data-label="operator">
                {event.actor_user_id ? <code>{event.actor_user_id}</code> : "平台自動"}
              </td>
              <td data-label="對象">
                {RESOURCE_LABEL[event.resource_type] ?? event.resource_type}{" "}
                <code>{event.resource_id ?? "不適用"}</code>
              </td>
              <td data-label="Workspace">
                {event.workspace_id ? <code>{event.workspace_id}</code> : "不適用"}
              </td>
              <td data-label="內容">
                <MetadataCell metadata={event.metadata} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
