import { Link } from "@tanstack/react-router";
import { useCreditLedger } from "../../admin.service";
import { Loading } from "../../../../shared/ui/Loading";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ENTRY_KIND } from "../../admin.model";
import { GrantForm } from "./GrantForm";

export function CreditPanel({ workspaceId }: { workspaceId: string }) {
  const ledger = useCreditLedger(workspaceId);
  return (
    <>
      <h2>點數</h2>
      {ledger.isPending && <Loading what="點數" />}
      <ReadFailure error={ledger.error} what="點數" />
      {ledger.data && (
        <>
          <p>
            目前餘額 <strong>{ledger.data.balance_credits}</strong> 點
          </p>
          {ledger.data.entries.length === 0 ? (
            <p>這個帳戶的分錄：0 筆。</p>
          ) : (
            <div className="table-scroll">
              <table>
                <caption>最近 50 筆分錄，新的在上面</caption>
                <thead>
                  <tr>
                    <th scope="col">時間</th>
                    <th scope="col">種類</th>
                    <th scope="col">點數</th>
                    <th scope="col">來源</th>
                  </tr>
                </thead>
                <tbody>
                  {ledger.data.entries.map((entry, index) => (
                    <tr key={`${entry.kind}-${index}`}>
                      <td>
                        <Timestamp at={entry.created_at} />
                      </td>
                      <td>
                        {ENTRY_KIND[entry.kind] ?? entry.kind}
                        {entry.estimated && "（估計值）"}
                      </td>
                      <td>
                        {entry.delta_credits > 0 ? `+${entry.delta_credits}` : entry.delta_credits}
                      </td>
                      <td>{entry.ref_type ?? "不適用"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="note">
            分錄不記授予的理由；誰在何時、以什麼理由授予，看{" "}
            <Link to="/admin/audit-log">動作紀錄</Link>。
          </p>
        </>
      )}
      <GrantForm workspaceId={workspaceId} />
    </>
  );
}
