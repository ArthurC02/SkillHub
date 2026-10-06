import { useRef, useState } from "react";
import { isUncertainGrantFailure, useGrantCredits } from "../../admin.service";
import { ActionForm } from "../../components/ActionForm";

export function GrantForm({
  workspaceId,
  ledgerReady,
}: {
  workspaceId: string;
  ledgerReady: boolean;
}) {
  const grant = useGrantCredits(workspaceId);
  const [amount, setAmount] = useState("");
  const [formVersion, setFormVersion] = useState(0);
  const submissionKey = useRef(crypto.randomUUID());
  const credits = Number(amount);
  const valid = amount.trim() !== "" && Number.isInteger(credits) && credits !== 0;
  const uncertain = grant.isError && isUncertainGrantFailure(grant.error);
  const startNewGrant = () => {
    grant.reset();
    setAmount("");
    submissionKey.current = crypto.randomUUID();
    setFormVersion((version) => version + 1);
  };
  return (
    <>
      <h3>授予點數</h3>
      <ActionForm
        key={formVersion}
        id="admin-grant"
        submitLabel="授予"
        pending={grant.isPending}
        error={uncertain ? undefined : grant.error}
        ready={valid && ledgerReady && !grant.isSuccess}
        readOnly={uncertain}
        unavailableReason={
          grant.isSuccess
            ? "這筆授予已完成；要再授予，請開始新的一筆。"
            : ledgerReady
              ? undefined
              : "先讀到目前點數狀態，才能授予。"
        }
        done={
          grant.data &&
          `已授予 ${grant.data.amount_credits} 點，授予時餘額為 ${grant.data.balance_credits} 點。`
        }
        contextKey={`${workspaceId}:${amount}`}
        onSubmit={(reason) =>
          grant.mutate({ amount_credits: credits, reason, idempotency_key: submissionKey.current })
        }
      >
        <div className="field">
          <label htmlFor="admin-grant-amount">點數（整數，不能是 0；負數是更正）</label>
          <input
            id="admin-grant-amount"
            type="number"
            step={1}
            value={amount}
            onChange={(event) => {
              setAmount(event.target.value);
              submissionKey.current = crypto.randomUUID();
              grant.reset();
            }}
            readOnly={grant.isPending || uncertain}
          />
        </div>
      </ActionForm>
      {grant.isSuccess && (
        <button type="button" disabled={!ledgerReady} autoFocus onClick={startNewGrant}>
          開始另一筆授予
        </button>
      )}
      {uncertain && (
        <div className="notice notice-warning" role="status">
          <p>
            這次授予的結果尚未確認。重試原授予會沿用同一筆識別，避免重複入帳；若要改金額或理由，先核對上方重新讀取的餘額與分錄。
          </p>
          <button type="button" disabled={!ledgerReady} onClick={startNewGrant}>
            已核對分錄，開始新授予
          </button>
        </div>
      )}
    </>
  );
}
