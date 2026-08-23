import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ApiError } from "../api/client";
import { useGenerateFailures, useGenerateSkill } from "../api/generate";
import { isCategorizedFindings } from "../api/import";
import type { GenerateRejected, GenerationFailure } from "../api/types";
import { Findings } from "./Findings";
import { GeneratedNotice } from "./GeneratedNotice";

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

/**
 * The three bounds a generation is held to, stated before the button is
 * pressed (02:GEN-001 「生成前顯示…本次將消耗的額度」, design system §2.2).
 *
 * These are not estimates and they are not the allowance. They are the ceilings
 * the server enforces — ErrGenerateTooLong in admission, finish_reason=length in
 * apps/llm, the retry loop's upper bound — and each carries a one-number marker
 * so `devctl automation-check` fails the day a copy drifts from the line that
 * enforces it. That marker is the answer to §3 第 11 條 「指出強制它的那一行」:
 * the line is named, and a machine compares against it.
 *
 * The textarea deliberately carries no `maxLength`: the browser counts UTF-16
 * code units and the server counts runes, so the same number would refuse an
 * emoji-heavy description the server accepts. One enforcer, and it is the
 * server; the 422 already says what to trim.
 *
 * What is NOT here is a dollar figure. The only unenforced input to a cost is
 * the unit price, which is the provider's fact and changes; printing the
 * measured average ($0.0055) as 「預估成本」 would be the promise §2.2 forbids
 * (04 丙-53). The allowance half is absent too, for the reason RunPreflight's
 * quota row is: a deployment enforcing none sends none, and a number would be a
 * claim (04 乙-2).
 */
const GENERATE_MAX_TASK_RUNES = 4000; // one-number: generateMaxTaskRunes
const GENERATE_MAX_OUTPUT_TOKENS = 16000; // one-number: generateMaxOutputTokens
const GENERATE_MAX_ATTEMPTS = 2; // one-number: generateMaxAttempts

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
      onSettled: async () => {
        // Both ways: a failure adds a row, and a success does not — but the
        // list is stale either way once a generation has been attempted.
        await queryClient.invalidateQueries({ queryKey: ["generate", "failures"] });
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
    <section>
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

      <dl>
        <dt>這一次最多會用到</dt>
        <dd>
          描述 {GENERATE_MAX_TASK_RUNES.toLocaleString("zh-TW")} 字、模型推理加輸出合計{" "}
          {GENERATE_MAX_OUTPUT_TOKENS.toLocaleString("zh-TW")} token、最多嘗試{" "}
          {GENERATE_MAX_ATTEMPTS} 次。
          <span className="note">
            {" "}
            這三個數字是伺服器實際擋你的上限，不是估計；超過第一個會被拒絕，超過第二個會直接停下、不重試。
          </span>
        </dd>
        <dt>預估成本</dt>
        <dd>
          尚未定值
          <span className="note">
            {" "}
            ——平台還沒有為單次生成設定費用上限，所以這裡不印一個沒有東西在保證的金額。
          </span>
        </dd>
      </dl>

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

      <GenerateHistory />
    </section>
  );
}

/**
 * GEN-003's read half: 「在工作區留下可查的失敗紀錄」.
 *
 * A separate clause from the verbatim findings shown above, which cover the
 * generation the user is looking at right now. This one has to outlive the
 * request — a failure the user walked away from is exactly the one they come
 * back to ask about.
 *
 * Absent when there is nothing to show, and that is not a §2.9 violation: an
 * empty history is a whole section with no rows, not a value position rendered
 * blank. A failed read IS a value position, and says so.
 */
function GenerateHistory() {
  const history = useGenerateFailures();

  if (history.isError) {
    return (
      <p className="note" role="status">
        過去的生成紀錄讀取失敗。這不影響你現在能不能生成。
      </p>
    );
  }
  const failures = history.data?.failures ?? [];
  if (failures.length === 0) return null;

  return (
    <details>
      <summary>最近沒有成功的生成（{failures.length} 次）</summary>
      <ul>
        {failures.map((f) => (
          <li key={f.occurred_at}>
            <time dateTime={f.occurred_at}>
              {new Date(f.occurred_at).toLocaleString("zh-TW")}
            </time>
            {" — "}
            {failureSentence(f)}
          </li>
        ))}
      </ul>
      <p className="note">
        這些是沒有建立任何版本的那幾次，最多列最近 20 次。
        <strong>這裡沒有記下你當時輸入的任務描述</strong>
        ——那份文字跟著它產生的 Skill 走，刪掉 Skill 就跟著刪掉；這份紀錄保存得更久，
        兩邊各留一份等於一個沒有人做過的保存承諾。
      </p>
    </details>
  );
}

/**
 * One row's sentence.
 *
 * Two of the five failures are things the user can act on — a truncated answer
 * means make the task smaller, a collision means rename the other skill — and
 * they are the two that get a next step. The other three say what happened and
 * stop, because 「再試一次」 is the only answer and it is already a button.
 */
function failureSentence(f: GenerationFailure): string {
  if (f.truncated) return "模型的輸出超過一次生成的上限，已經停下。把任務拆小一點再試會有幫助。";
  if (f.collision) return "工作區已經有一個同名的 Skill，而它不是生成的。改掉那一個的名字或刪掉它再試。";
  switch (f.failure) {
    case "quota":
      return "額度不足，沒有呼叫模型，也沒有花錢。";
    case "unavailable":
      // Not "額度不足": the allowance could not be counted, and a healthy
      // account must not be told it ran out (d555564 fixed the 422; this is
      // the same sentence on the record).
      return "當時算不出剩餘額度，平台沒有冒險呼叫模型，也沒有花錢。";
    case "gateway":
      return "模型服務沒有回應。沒有建立任何版本。";
    case "unpackageable":
      return "模型交出來的東西沒辦法打包成一個套件。";
    case "rejected":
      return "驗證之後被拒絕，沒有建立任何版本。";
    case "blocked":
      return f.codes?.length
        ? `套件沒有通過驗證（${f.codes.join("、")}），試了 ${f.attempts} 次。`
        : `套件沒有通過驗證，試了 ${f.attempts} 次。`;
    default:
      // The row exists and its timestamp is real; only its detail is missing.
      // Saying so beats dropping the row, which would silently shorten history.
      return "這一次沒有成功，而這列紀錄的細節讀不出來。";
  }
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
    <div role="status" className="notice">
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
 * The two absences come from GeneratedNotice rather than being written again
 * here. They were written again here, and the copy had already started to
 * drift: it said 「這份內容是模型剛剛寫出來的」 where the detail page says 「平台
 * 依任務描述生成的」, and it dropped the link to the first trial run. Findings.tsx
 * names the rule this broke — two copies is how one of the two screens quietly
 * rewrites a warning into reassurance.
 *
 * `attempts` stays local, because it belongs to this generation and not to the
 * package: 02:GEN-003 forbids the UI promising a success rate — one retry was
 * measured to move 80% to 90%, not to nothing — and "it took two goes" is the
 * honest form of that number.
 */
function GenerateSucceeded({
  result,
}: {
  result: { skill_id: string; version_number: number; attempts: number };
}) {
  return (
    <section role="status">
      <h3>已經產生一個 Skill，放在你的工作區</h3>
      <GeneratedNotice skillId={result.skill_id} />
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
