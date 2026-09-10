import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useCancelAccountDeletion, useMe, useRequestAccountDeletion } from "../api/me";
import { ConfirmDelete } from "../components/ConfirmDelete";

function deletionFailureSentence(error: unknown): string {
  if (error instanceof ApiError && error.status === 409) return "刪除已經不可逆，無法再變更。";
  return "這個要求沒有記錄成功，可以再按一次。";
}

export function WorkspaceAccount() {
  const me = useMe();
  const client = useQueryClient();
  const [message, setMessage] = useState("");

  const request = useRequestAccountDeletion();
  const cancel = useCancelAccountDeletion();

  const refresh = () => client.invalidateQueries({ queryKey: ["me"] });

  return (
    <section>
      <h1>帳號</h1>

      {me.isPending && <Loading what="帳號資料" />}
      <ReadFailure error={me.error} what="帳號資料" />
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
          <p className="note">
            {me.data.display_name}（{me.data.email}）
          </p>
          <details>
            <summary>工作區識別碼</summary>
            <code>{me.data.workspace_id}</code>
          </details>

          <h2>刪除帳號</h2>
          {me.data.deletion_requested_at ? (
            <PendingDeletion
              requestedAt={me.data.deletion_requested_at}
              purgeAfter={me.data.purge_after}
              scope={me.data.deletion_scope}
              pending={cancel.isPending}
              onCancel={() =>
                cancel.mutate(undefined, {
                  onSuccess: async () => {
                    setMessage("已取消。帳號不會被刪除，資料照舊。");
                    await refresh();
                  },
                })
              }
            />
          ) : (
            <p>
              <ConfirmDelete
                scopeId="account-delete-scope"
                label="刪除我的帳號"
                confirmLabel="確認開始刪除"
                pending={request.isPending}
                onAsk={() => setMessage("")}
                onConfirm={() =>
                  request.mutate(undefined, {
                    onSuccess: async (result) => {
                      setMessage(result.scope);
                      await refresh();
                    },
                  })
                }
                scope={
                  <>
                    這一步<strong>不會立刻刪掉任何東西</strong>
                    ：它開始一段寬限期，期間帳號照常可以用，隨時可以取消。
                    按下之後伺服器會回覆「哪些會刪、哪些會保留但去掉你的身分」的完整說明，
                    以及寬限期結束的日期；在那之前都還來得及反悔。
                  </>
                }
              />
            </p>
          )}

          <p className="note" data-role="teaching">
            只想刪掉某幾樣東西，不想刪帳號？哪一樣刪在哪裡、刪掉之後什麼會留下，見
            <Link to="/policy">資料保存政策</Link>。
          </p>
        </>
      )}
    </section>
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
        在那之前帳號照常可以用，這個申請也隨時可以取消。再按一次刪除不會提早，也不會重新計時。
      </p>
      <p>
        <button type="button" disabled={pending} onClick={onCancel}>
          取消刪除申請
        </button>
      </p>
    </>
  );
}
