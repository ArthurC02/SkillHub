import { useRef, useState } from "react";
import { useGrantCredits } from "../../admin.service";
import { ActionForm } from "../../components/ActionForm";

export function GrantForm({ workspaceId }: { workspaceId: string }) {
  const grant = useGrantCredits(workspaceId);
  const [amount, setAmount] = useState("");
  const submissionKey = useRef(crypto.randomUUID());
  const credits = Number(amount);
  const valid = amount.trim() !== "" && Number.isInteger(credits) && credits !== 0;
  return (
    <>
      <h3>授予點數</h3>
      <ActionForm
        id="admin-grant"
        submitLabel="授予"
        pending={grant.isPending}
        error={grant.error}
        ready={valid}
        done={
          grant.data &&
          `已授予 ${grant.data.amount_credits} 點，餘額現在是 ${grant.data.balance_credits} 點。`
        }
        contextKey={`${workspaceId}:${amount}`}
        onSubmit={(reason) =>
          grant.mutate(
            { amount_credits: credits, reason, idempotency_key: submissionKey.current },
            {
              onSuccess: () => {
                submissionKey.current = crypto.randomUUID();
              },
            },
          )
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
            readOnly={grant.isPending}
          />
        </div>
      </ActionForm>
    </>
  );
}
