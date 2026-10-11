import { useState } from "react";
import { useUpdateTestCase } from "../../testcases.service";
import type { TestCase } from "../../testcases.service";
import { MutationError } from "./MutationError";

const MAX_FACTS = 10;

function factsOf(text: string) {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");
}

export function SimulatedUserForm({ testCase }: { testCase: TestCase }) {
  const [text, setText] = useState(testCase.simulated_user_facts.join("\n"));
  const save = useUpdateTestCase(testCase.test_case_id);
  const facts = factsOf(text);
  const tooMany = facts.length > MAX_FACTS;

  return (
    <>
      <h2>模擬使用者</h2>
      <p className="note">
        寫下這位使用者知道、但在 User Prompt 裡沒說的事，一行一件。批次評估時，試跑若停下來問問題，
        平台會以這位使用者的身分把這些事告訴它，讓對話繼續；留白則由真人回答。
      </p>
      <p className="field">
        <label htmlFor="simulated-user-facts">使用者知道的事</label>
        <textarea
          id="simulated-user-facts"
          rows={4}
          cols={60}
          value={text}
          onChange={(e) => {
            save.reset();
            setText(e.target.value);
          }}
          readOnly={save.isPending}
          aria-describedby="simulated-user-limit"
        />
        <br />
        <span id="simulated-user-limit" className="note" role={tooMany ? "status" : undefined}>
          {tooMany
            ? `目前 ${facts.length} 行，最多 ${MAX_FACTS} 行。`
            : `最多 ${MAX_FACTS} 行，合計最多 1500 bytes。`}
        </span>
      </p>
      <button
        type="button"
        disabled={save.isPending || tooMany}
        onClick={() => save.mutate({ simulated_user_facts: facts })}
      >
        {save.isPending ? "儲存中…" : "儲存模擬使用者"}
      </button>
      {save.isSuccess && <p role="status">已儲存。</p>}
      <MutationError error={save.error} what="模擬使用者" fallback="儲存沒有成功，可以再試一次。" />
    </>
  );
}
