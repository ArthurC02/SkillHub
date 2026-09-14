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
        <dt>封測名單</dt>
        <dd>{account.in_beta_allowlist ? "在名單上，或這個部署沒有設定名單" : "不在名單上"}</dd>
      </dl>
      <CreditPanel workspaceId={account.workspace_id} />
    </>
  );
}
