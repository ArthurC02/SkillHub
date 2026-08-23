import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useGenerateSkill } from "../api/generate";
import { isCategorizedFindings } from "../api/import";
import type { GenerateRejected } from "../api/types";
import { Findings } from "./Findings";

/**
 * GEN-004's entry point: describe the task, get a Skill package (GEN-001).
 *
 * Rendered in two places and nowhere else — the search's no-results state and
 * the workspace's own Skill list. **Not on the home page beside search**:
 * ADR-046 決策 7 makes 「先搜尋、搜不到再生成」 a product opinion, and an entry
 * point of equal weight next to the search box says the opposite.
 *
 * `useGenerateEntryPoint` is what decides whether this exists at all. Everything
 * here assumes the flag is already on.
 */

export function GenerateSkill({ initialTask = "" }: { initialTask?: string }) {
  const [task, setTask] = useState(initialTask);
  const [rejected, setRejected] = useState<GenerateRejected>();
  const queryClient = useQueryClient();
  const mutation = useGenerateSkill();

  const submit = () => {
    setRejected(undefined);
    mutation.mutate(task, {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: ["skills"] });
      },
      onError: (error) => {
        // The categorised 422 is the package's own findings, verbatim, exactly
        // as a failed import renders them (02:GEN-003). The uncategorised one is
        // a refusal around the model call — blank input, no allowance left,
        // output truncated — and has a sentence instead.
        setRejected(
          error instanceof ApiError && isCategorizedFindings(error.body)
            ? (error.body as GenerateRejected)
            : undefined,
        );
      },
    });
  };

  return (
    <section className="generate-skill">
      <h2>沒有夠接近的？讓平台依你的描述做一個</h2>
      <p className="note">
        平台會依你寫的任務描述產生一個 Skill 套件，放進你自己的工作區。
        它<strong>不會進入公開目錄，也不會出現在搜尋結果裡</strong>——包括你自己搜尋的時候。
      </p>

      <label htmlFor="generate-task">任務描述</label>
      <textarea
        id="generate-task"
        rows={4}
        value={task}
        onChange={(e) => setTask(e.target.value)}
        placeholder="要完成什麼、輸入是什麼、預期產出是什麼。"
        disabled={mutation.isPending}
      />

      <button type="button" onClick={submit} disabled={mutation.isPending || task.trim() === ""}>
        {mutation.isPending ? "生成中…" : "生成一個 Skill"}
      </button>

      {mutation.isPending && <GenerateInFlight />}

      {/*
        A refusal the server did not categorise: blank description, no allowance
        left, or output past the token ceiling. The message is the whole answer,
        and each of those three already says what to do next.
      */}
      {mutation.error && !rejected && <p role="alert">{mutation.error.message}</p>}

      {rejected && <GenerateFailed rejected={rejected} onRetry={submit} />}

      {mutation.data && <GenerateSucceeded result={mutation.data} />}
    </section>
  );
}

/**
 * Design system §2.12: an in-progress screen has to say which step it is on,
 * whether the step ends by itself, and **whether the user can leave**.
 *
 * The third answer is different from a Run's, and it is the reason this is not
 * the `InFlight` component. A Run is a River job consumed by cmd/worker and its
 * trace arrives by POST from the sandbox — the browser is in neither path, so
 * closing the tab is safe. A generation is one synchronous request: there is no
 * job, no run id, and an abandoned request is a cancelled generation. §2.2
 * forbids showing a promise nothing enforces, and "you can close this" would be
 * exactly that.
 */
function GenerateInFlight() {
  return (
    <div role="status" className="in-flight">
      <p>正在請模型寫這個 Skill，然後用與匯入完全相同的那道驗證檢查它。</p>
      <p>這一步會自己結束，通常十幾秒到一分鐘。</p>
      <p>
        <strong>請不要關掉這個分頁</strong>
        ——這一次生成沒有背景工作可以接手，關掉就等於取消，而且不會留下任何半成品版本。
      </p>
    </div>
  );
}

/**
 * 02:GEN-003: the findings go to the user verbatim, not rewritten into
 * reassurance, and there are two named ways out. Nothing was stored — the
 * validation happens before the object store write and before the transaction —
 * so there is no half-made version to clean up, and saying so is part of the
 * answer.
 */
function GenerateFailed({ rejected, onRetry }: { rejected: GenerateRejected; onRetry: () => void }) {
  return (
    <section role="alert">
      <h3>生成失敗：套件被擋下，沒有建立任何版本</h3>
      {/*
        Conditional, because the automatic retry does not always happen: a
        `possible-secret` finding is not retried (ADR-048), and saying "we
        already tried twice" about one attempt is the same §2.2 violation this
        file's in-flight block refuses to commit one component up. The server
        sends `attempts` on the 422 for exactly this sentence.
      */}
      <p className="note">
        {rejected.attempts > 1
          ? "平台已經自動用同一段描述再試過一次，第二次仍然沒有通過。下面是檢查逐字回報的內容，沒有經過改寫。"
          : "這一次沒有自動重試——被擋下的原因不是排版手滑，同一段描述再送一次會得到同樣的結果。下面是檢查逐字回報的內容，沒有經過改寫。"}
      </p>
      <Findings findings={rejected} />
      <p>
        <button type="button" onClick={onRetry}>
          再試一次
        </button>
        {" "}
        或者改寫上面的任務描述再送出——把要做什麼、輸入是什麼、預期產出是什麼寫得更具體，通常比重試有用。
      </p>
    </section>
  );
}

/**
 * GEN-004: what the user must be told about a package nobody has looked at.
 *
 * `attempts` is shown because 02:GEN-003 forbids the UI promising a success
 * rate — one retry was measured to move 80% to 90%, not to nothing — and
 * "it took two goes" is the honest form of that number.
 */
function GenerateSucceeded({
  result,
}: {
  result: { skill_id: string; version_number: number; attempts: number };
}) {
  return (
    <section role="status">
      <h3>已經產生一個 Skill，放在你的工作區</h3>
      <p className="badge badge-unverified">
        沒有經過任何人工檢視，沒有任何試跑證據
      </p>
      <p className="note">
        這份內容是模型剛剛寫出來的。它通過的只有格式與靜態檢查，
        <strong>那不是品質、可用性或安全的結論</strong>。
        先跑一次試跑，才會有第一份證據。
      </p>
      {result.attempts > 1 && (
        <p className="note">這一次生成試了 {result.attempts} 趟才通過驗證。</p>
      )}
      <p>
        <Link to="/skills/$skillId" params={{ skillId: result.skill_id }}>
          打開這個 Skill
        </Link>
      </p>
    </section>
  );
}
