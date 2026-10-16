import { Loading } from "../../../shared/ui/Loading";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { PageReadFailure, ReadFailure } from "../../../shared/ui/LoginRequired";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../../../core/api/client";
import {
  useCancelAccountDeletion,
  useMe,
  useRequestAccountDeletion,
} from "../../../core/session/me.service";
import { ConfirmDelete } from "../../../shared/ui/ConfirmDelete";
import { CreditStatement } from "./CreditStatement";
import "./WorkspaceAccount.page.css";

function deletionFailureSentence(error: unknown): string {
  if (error instanceof ApiError && error.status === 409) return "刪除已經不可逆，無法再變更。";
  return "未能取得操作結果，請以最新帳號狀態確認是否需要再操作。";
}

export function WorkspaceAccount() {
  const me = useMe();
  const [message, setMessage] = useState("");
  const request = useRequestAccountDeletion();
  const cancel = useCancelAccountDeletion();

  if (me.error) {
    return (
      <AccountReadFailure
        query={me}
        accepted={request.isSuccess || cancel.isSuccess}
        unconfirmed={request.isError || cancel.isError}
      />
    );
  }

  return (
    <section className="account-page">
      <h1>帳號</h1>

      {me.isPending && <Loading what="帳號資料" />}
      {!me.isPending && (request.isPending || cancel.isPending || me.isFetching) && (
        <p role="status">正在確認最新帳號狀態…</p>
      )}
      {message && <p role="status">{message}</p>}
      {request.error && (
        <ReadFailure error={request.error} what="帳號刪除申請">
          <p role="alert">{deletionFailureSentence(request.error)}</p>
        </ReadFailure>
      )}
      {cancel.error && (
        <ReadFailure error={cancel.error} what="取消刪除申請">
          <p role="alert">{deletionFailureSentence(cancel.error)}</p>
        </ReadFailure>
      )}

      {me.data && (
        <>
          <div className="account-profile">
            <p>
              <strong>{me.data.display_name}</strong>
              <span>{me.data.email}</span>
            </p>
            <details>
              <summary>工作區識別碼</summary>
              <code>{me.data.workspace_id}</code>
            </details>
          </div>

          <div className="account-settings-grid">
            <section className="account-panel">
              <CreditStatement />
            </section>
          </div>

          <section className="account-panel account-danger-zone">
            <h2>刪除帳號</h2>
            {me.data.deletion_requested_at ? (
              <PendingDeletion
                requestedAt={me.data.deletion_requested_at}
                purgeAfter={me.data.purge_after}
                scope={me.data.deletion_scope}
                pending={cancel.isPending}
                onCancel={() => {
                  request.reset();
                  setMessage("");
                  cancel.mutate(undefined, {
                    onSuccess: () => setMessage("已取消。帳號不會被刪除，資料照舊。"),
                  });
                }}
              />
            ) : (
              <p data-role="reason">
                {!request.isPending && !cancel.isPending && !me.isFetching && (
                  <span role="status">目前沒有刪除申請。</span>
                )}
                <ConfirmDelete
                  scopeId="account-delete-scope"
                  label="刪除我的帳號"
                  confirmLabel="確認開始刪除"
                  pending={request.isPending}
                  onAsk={() => setMessage("")}
                  onConfirm={() => {
                    cancel.reset();
                    request.mutate(undefined, {
                      onSuccess: (result) => setMessage(result.scope),
                    });
                  }}
                  scope={
                    <>
                      這一步<strong>不會立刻刪掉任何東西</strong>
                      ：它開始一段寬限期，期間帳號照常可以用；寬限期內可以提出取消，
                      能否取消以伺服器收到操作時的狀態為準。
                      按下之後伺服器會回覆「哪些會刪、哪些會保留但去掉你的身分」的完整說明，
                      以及寬限期結束的日期。
                    </>
                  }
                />
              </p>
            )}
            <p>
              <Link to="/policy">個別資料的保存與刪除政策</Link>
            </p>
          </section>
        </>
      )}
    </section>
  );
}

function AccountReadFailure({
  query,
  accepted,
  unconfirmed,
}: {
  query: ReturnType<typeof useMe>;
  accepted: boolean;
  unconfirmed: boolean;
}) {
  return (
    <PageReadFailure
      heading="帳號"
      error={query.error}
      what="帳號資料"
      retry={() => void query.refetch()}
      retrying={query.isFetching}
    >
      {(accepted || unconfirmed) && (
        <p role="alert">
          {unconfirmed
            ? "未能取得操作結果，最新帳號狀態也暫時無法確認。"
            : "操作已送出，但最新帳號狀態暫時無法確認。"}
          再試一次只重新讀取狀態，不會重複送出操作。
        </p>
      )}
    </PageReadFailure>
  );
}

function PendingDeletion({
  requestedAt,
  purgeAfter,
  scope,
  pending,
  onCancel,
}: {
  requestedAt: string;
  purgeAfter: string | null;
  scope: string | null;
  pending: boolean;
  onCancel: () => void;
}) {
  return (
    <>
      <p role="status">
        <span className="badge badge-danger">刪除申請中</span> 申請於 <Timestamp at={requestedAt} />
        。
        {purgeAfter ? (
          <>
            寬限期在 <Timestamp at={purgeAfter} /> 結束，之後才會真的刪除。
          </>
        ) : (
          "未測量——伺服器沒有回報寬限期結束的日期，那不表示沒有期限，是這一頁問不到它。"
        )}
      </p>
      {scope ? (
        <p>{scope}</p>
      ) : (
        <p className="note">
          未測量——伺服器沒有附上刪除範圍的說明。這不代表範圍是空的，代表這一頁現在讀不到它。
        </p>
      )}
      <p className="note">
        你可以嘗試取消這個申請；能否取消，以伺服器收到操作時的狀態為準。若刪除已進入不可逆階段，
        這一頁會明確告知。再按一次刪除不會提早，也不會重新計時。
      </p>
      <p>
        <button type="button" disabled={pending} onClick={onCancel}>
          取消刪除申請
        </button>
      </p>
    </>
  );
}
