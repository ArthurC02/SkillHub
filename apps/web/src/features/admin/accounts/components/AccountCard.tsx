import type { AccountLookup } from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { CreditPanel } from "./CreditPanel";

export function AccountCard({ account }: { account: AccountLookup }) {
  return (
    <>
      <h2>{account.display_name}</h2>
      <dl>
        <dt>Email</dt>
        <dd>{account.email}</dd>
        <dt>刪除申請</dt>
        <dd>
          {account.deletion_requested_at ? (
            <>
              已申請，時間 <Timestamp at={account.deletion_requested_at} />
            </>
          ) : (
            "沒有申請"
          )}
        </dd>
        <dt>封測准入</dt>
        <dd>
          {account.in_beta_allowlist
            ? "目前可通過；可能是已受邀，或此部署未限制。"
            : "目前不可通過；請確認封測名單或部署設定。"}
        </dd>
      </dl>
      <details>
        <summary>帳號識別資料與建立時間</summary>
        <dl>
          <dt>User id</dt>
          <dd>
            <code>{account.user_id}</code>
          </dd>
          <dt>Workspace id</dt>
          <dd>
            <code>{account.workspace_id}</code>
          </dd>
          <dt>建立時間</dt>
          <dd>
            <Timestamp at={account.created_at} />
          </dd>
        </dl>
      </details>
      <CreditPanel workspaceId={account.workspace_id} />
    </>
  );
}
