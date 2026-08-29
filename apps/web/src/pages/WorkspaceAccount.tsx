import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { ReadFailure } from "../components/LoginRequired";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useCancelAccountDeletion, useMe, useRequestAccountDeletion } from "../api/me";
import { ConfirmDelete } from "../components/ConfirmDelete";

/**
 * CORE-007 / 02:SEC-006 — the account, and the second of the two deletion planes
 * (04 丙-22). The first plane deletes one thing at a time: a package, a run's
 * output, a skill. This one ends the account, and the two are not versions of
 * each other — the per-item deletes take effect at once, this one starts a grace
 * period and takes effect at the end of it. How long that period lasts is not
 * written here: PDM-006 is unratified, so a numeral on this side would be a
 * promise nobody has ratified (design §2.2). The server's `purge_after` date is
 * the only figure this screen states.
 *
 * Three endpoints have existed since M1 with no screen on them, which is the same
 * 尺-1 shape the workspace lists were: 02:SEC-006 asks for 刪除工作具可追蹤狀態,
 * and a state nobody can look at is not one a user can follow.
 *
 * The scope copy is deliberately split in two, and which half comes from where is
 * the point:
 *
 *  - **Before**, this page says what pressing the button does — it starts a
 *    countdown, deletes nothing yet, and is cancellable throughout. That is a
 *    fact about *this control*, so it belongs to the control.
 *  - **After**, the server's own `scope` sentence is shown verbatim. What is
 *    destroyed and what is kept de-identified is a fact about the platform's
 *    deletion job, and the API owns that wording (WS-002/PDM-006 §6.1). A second
 *    copy on this side would be a second thing to keep true, and the copy that
 *    goes stale is always the one further from the job.
 *
 * That split used to come with a hole, and it is worth recording how it closed.
 * `scope` reached the screen only in the response to DELETE /me, so it lived in
 * component state: a reload lost the disclosure while the grace period it
 * described ran on, and the reader who goes looking for it is precisely the one
 * who came back later. The page was made to stop promising what it could not
 * keep, which was honest but not a fix. `Me.deletion_scope` (04 丙-30) is the
 * fix — one constant, both endpoints, so the sentence cannot go stale on one
 * side of the pair.
 */
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
      {/*
        資訊架構 §5 IA-6 listed eleven routes; this is the twelfth. The sentence
        that used to sit here — 「沒有登入的話，這一頁不會有東西可以看——那不是讀取
        失敗。」 — is cited in that ruling as one of the three precedents for
        「由頁面自己說」, and it was right about the state. It was still printing
        「無法讀取帳號資料：not authenticated」 in front of itself, and it named no
        way to log in. The shared component says the same thing in the product's
        own language and carries the action; the trailing clause is now the whole
        message rather than a footnote to an English one.

        A non-401 keeps 「無法讀取帳號資料：{message}」 and does NOT keep the login
        clause: a 500 is not somebody being logged out.
      */}
      <ReadFailure error={me.error} what="帳號資料" />
      {message && <p role="status">{message}</p>}

      {me.data && (
        <>
          <p className="note">
            {me.data.display_name}（{me.data.email}）｜工作區 <code>{me.data.workspace_id}</code>
          </p>

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
                  onError: (err) =>
                    setMessage(err instanceof Error ? err.message : "取消失敗，請再試一次。"),
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
                    onError: (err) =>
                      setMessage(err instanceof Error ? err.message : "刪除申請失敗，請再試一次。"),
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

          <p className="note">
            只想刪掉某幾樣東西，不想刪帳號？打包好的檔案在
            <Link to="/workspace/downloads">下載紀錄</Link>，一次跑的產出在該次
            <Link to="/workspace/runs">Run</Link> 的頁面，Skill 本身在
            <Link to="/workspace/skills">我的 Skill</Link>。 各類資料保存多久見
            <Link to="/policy">資料保存政策</Link>。
          </p>
        </>
      )}
    </section>
  );
}

/**
 * The requested-but-not-yet-purged state. It is a state and not a receipt: the
 * date the grace period ends is on screen, and so is the way out of it, because
 * a user who closed the tab after asking has no other place to find either.
 */
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
          "伺服器沒有回報寬限期結束的日期——那不表示沒有期限，是這一頁問不到它。"
        )}
      </p>
      {/*
       * 設計 §2.8: the scope sentence is the whole disclosure, and §2.10 puts it
       * on the never-collapse list. Verbatim from the server (WS-002/PDM-006
       * §6.1 own the wording); a second copy here would be a second thing to
       * keep true, and the stale one is always the copy further from the job.
       */}
      {scope ? (
        <p>{scope}</p>
      ) : (
        <p className="note">
          伺服器沒有附上刪除範圍的說明。這不代表範圍是空的——代表這一頁現在讀不到它。
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
