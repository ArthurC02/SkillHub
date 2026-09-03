import { Loading } from "../components/Loading";
import { Timestamp } from "../components/Timestamp";
import { LoginRequired, ReadFailure, unauthenticated } from "../components/LoginRequired";
import { useMe } from "../api/me";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useRunComparison, useVersionDiff } from "../api/evaluation";
import type { ComparisonSide, RunComparison } from "../api/evaluation";
import { useRun, useRuns } from "../api/runs";
import { RunVerdict } from "../components/RunVerdict";
import { CRITERION_LABEL, OVERALL_LABEL, runStatusLabel } from "./RunEvaluation";

/**
 * 02:EVAL-003 — two runs side by side.
 *
 * A read and nothing else: both sides are frozen snapshots, so nothing here can
 * change a historical run (iron rule 4). Three things the layout has to get
 * right:
 *
 * 1. **Execution and judgement stay in separate rows.** A side at `succeeded`
 *    with no evaluation shows 執行完成 and 未評估 — never a pass inferred from
 *    the terminal state (ADR-025).
 * 2. **The run's cost is a lower bound and says so** (丙-3). The evaluation's
 *    own cost is a second row, never added into the first.
 * 3. **`inputs_available: false` removes the re-run affordance.** The inputs
 *    were deleted or expired, so a screen offering to run them again would be
 *    offering something the platform cannot do (ADR-003).
 *
 * There is no re-run button even when the inputs are there. What the side's
 * `skill_id` / `skill_version_id` / `test_case_id` buy is a link to the preflight
 * screen with those three filled in — the user still reads the permission summary
 * and confirms its hash there, which is the whole of TEST-009. A button that
 * started the run from here would be the shortcut around it.
 */

/*
 * OVERALL_LABEL and CRITERION_LABEL are imported, not re-declared. This file
 * used to keep its own `Record<string, string>` copies of both: the originals
 * are keyed on the discriminated union, so a new server enum value was a
 * compile error there and a silent `undefined` here — the two planes wording
 * one fact two ways, which is what 02:NFR-001 forbids.
 */

function verdictCell(side: ComparisonSide) {
  if (!side.evaluation) return "未評估（不是通過）";
  if (side.evaluation.status === "failed") return "評估未完成";
  if (side.evaluation.status === "pending") return "評估進行中";
  return OVERALL_LABEL[side.evaluation.overall] ?? side.evaluation.overall;
}

/**
 * 設計 §2.13 去重 1 ＋ §4.3：**表格裡的但書屬於欄／列，不屬於格。**
 *
 * 這一句在左右兩格逐位元相同地印了兩次，而一句在每一格上都一樣的話，讀者從第二格起
 * 不可能因為它而作出不同判斷——它講的是這一列（Run 成本）是什麼，不是這一格是什麼。
 * 所以它搬到列首，那裡已經寫著「Run 成本（下界）」。
 *
 * 兩側**真的不同**時規則不觸發（不同的權威來源，或只有一側是下界），這時每一格各自
 * 留著自己的那一句——那才是能區分這一格與那一格的事實，§2.10 保護的正是它。
 */
function costNote(side: ComparisonSide): string {
  return `${side.cost.is_lower_bound ? "這是下界，不是總額。" : ""}權威來源：${
    side.cost.authoritative_source
  }`;
}

function usd(value: number | null): string {
  // 設計 §2.9 的表列詞;閘道沒有回報一個成本，不是 0。
  return value === null ? "未測量" : `US$${value.toFixed(4)}`;
}

export function RunCompare() {
  const { runId } = useParams({ from: "/runs/$runId/compare" });
  const { against = "" } = useSearch({ strict: false }) as { against?: string };
  const [draft, setDraft] = useState(against);
  useEffect(() => setDraft(against), [against]);
  const navigate = useNavigate();
  const comparison = useRunComparison(runId, against);
  // 資訊架構 §5 IA-6, the second deferred-rejection page: with `against` empty
  // this screen fires no request at all, so nothing would have told a visitor
  // anything until after they had found and pasted a 36-char run id. Same
  // `useMe()` shape as ImportSkill and for the same reason — a resolved 401,
  // never a pending one.
  const me = useMe();
  const loggedOut = unauthenticated(me.error);

  /*
   * The door handle. This screen was complete except that using it began with
   * finding a 36-char uuid on another page and pasting it in — so the candidates
   * are read here instead: the runs of the same Test Case this run was frozen
   * from (GET /runs?test_case_id=, WS-004).
   *
   * Everything below is still a read. Picking a candidate changes which existing
   * run is fetched and nothing else; there is deliberately no re-run here, for
   * the reason the comparison handler's own header gives — a second way to start
   * a run is a way around the permission confirmation of TEST-009.
   */
  const self = useRun(runId);
  const testCaseId = self.data?.test_case_id;
  const siblings = useRuns(testCaseId, Boolean(testCaseId));
  // Self-comparison is a 400 from the server, so this side is not a candidate.
  // Gated on `testCaseId` as well: until it resolves the list is the whole
  // workspace's history, which is not the same question.
  const candidates = testCaseId
    ? (siblings.data?.pages.flatMap((p) => p.runs) ?? []).filter((r) => r.run_id !== runId)
    : [];

  // The selection lives in the URL so a comparison is linkable, the same rule
  // the Explorer's compare screen follows — the picker only writes it.
  const pick = (id: string) =>
    void navigate({ to: "/runs/$runId/compare", params: { runId }, search: { against: id } });

  const pickForm = (
    <>
      {self.isPending ? (
        <Loading what="目前這次 Run" />
      ) : self.error ? (
        <ReadFailure error={self.error} what="目前這次 Run" />
      ) : !testCaseId ? (
        <p>這次 Run 的 Test Case 已無法解析，因此無法列出同一個 Test Case 的其他 Run。</p>
      ) : siblings.isPending ? (
        <Loading what="可比較的 Run" />
      ) : siblings.error ? (
        <ReadFailure error={siblings.error} what="可比較的 Run" />
      ) : candidates.length > 0 ? (
        // Same two axes and the same order as the other two run histories
        // (WorkspaceRuns, TestCases): 任務判定 first, 執行狀態 second, time last.
        // No uuid on the row — not showing one is the entire point of this list.
        <ul className="download-list">
          {candidates.map((r) => (
            <li key={r.run_id} className="download-item">
              <p className="badge-row">
                <RunVerdict verdict={r.evaluation} />
              </p>
              <p className="badge-row">
                <span className="badge">執行狀態：{runStatusLabel(r.status)}</span>
              </p>
              {r.status_reason && <p className="note">{r.status_reason}</p>}
              <p>
                <button type="button" onClick={() => pick(r.run_id)}>
                  與這一次比較（建立於 <Timestamp at={r.created_at} />）
                </button>
              </p>
            </li>
          ))}
        </ul>
      ) : (
        // §2.4: a control that is gone has to say why. The paste box stays either
        // way — it is the only route to a run of another Test Case or Skill.
        <p>這個 Test Case 目前只有這一次 Run，沒有同一個 Test Case 的其他 Run 可選。</p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          pick(draft);
        }}
      >
        <label htmlFor="against">要比較的另一個 Run ID</label>{" "}
        <input
          id="against"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          size={40}
          placeholder="另一個 Run 的平台 run_id"
        />{" "}
        <button type="submit">比較</button>
        {/*
          設計 §2.13 去重 2（同頁同義句）：挑另一個 Run 這件事以前在同一屏上講三次——
          這裡一次、清單底下的「從上面選一個…」一次、沒有候選時的「輸入另一個 Run 的
          ID…」一次。三句合成一句，留在它描述的那個控制項旁邊；候選清單存在時才多出
          「從上面選一個」那半句，因為那半句在沒有候選時是假的。
        */}
        <p className="note">
          {(candidates.length > 0 ? "從上面選一個同一個 Test Case 的 Run，或" : "") +
            "輸入另一個 Run 的 ID 後開始比較。別的 Test Case 或別的 Skill 的 Run 也可以。"}
        </p>
      </form>
    </>
  );

  return (
    <section>
      <h1>Run 比較</h1>

      {/* Checklist 1: the page led with a uuid, then a form, then two columns
          headed by 36-char ids, and no sentence anywhere said what differed.
          Both verdicts were already computed; now they are the headline. */}
      {comparison.data && <ComparisonLead data={comparison.data} />}
      {/*
        以前這裡有一句「比較只是讀取，不會改動任何一個 Run 的歷史資料。」，2026-09-03
        依設計 §2.13 移除：這一頁沒有任何寫入控制項，那句話回答的是沒有人問的問題。
        §2.4 管的是**被停用或被拿掉的控制項**要說原因——這裡沒有這樣的控制項，「不能
        改」不是一個缺席的功能，是這一頁本來的形狀。真正需要說原因的那一處仍然在：
        RerunCell 說得出為什麼沒有「重跑」按鈕。改成 h1 旁一個「（唯讀）」也不誠實——
        那個字讀起來像「你沒有寫入權限」，而不變的是 Run 快照本身（鐵律 4），跟看的
        人是誰無關。檔頭的 "A read and nothing else" 仍然是維護者要看的那份紀錄。
      */}

      {/* Design §2.6: the two run ids are identifiers, not the answer. */}
      <details>
        <summary>進階資訊（Run 識別碼）</summary>
        <ul>
          <li>
            這一邊：<code>{runId}</code>
          </li>
          <li>另一邊：{against === "" ? "尚未選擇" : <code>{against}</code>}</li>
        </ul>
      </details>

      {/* With a comparison on screen the picker is a "change it" affordance and
          its value is a 36-char uuid, so it folds (design §2.6). With nothing
          picked yet it is the only way forward and stays open. */}
      {loggedOut ? (
        // Replaced rather than disabled (§2.4): a control taken away has to say
        // why, and 「比較」 needs both runs' evaluations, which are workspace data.
        <LoginRequired what="Run 比較" />
      ) : comparison.data ? (
        <details>
          <summary>換一個要比較的 Run</summary>
          {pickForm}
        </details>
      ) : (
        pickForm
      )}

      {comparison.isPending && against !== "" && <Loading what="比較" />}
      <ReadFailure error={comparison.error} what="比較結果">
        <p role="alert">無法比較：{comparison.error?.message}</p>
      </ReadFailure>
      {comparison.data && <ComparisonTables data={comparison.data} />}

      <p className="note">
        <Link to="/runs/$runId" params={{ runId }}>
          回到這個 Run 的詳情
        </Link>
      </p>
    </section>
  );
}

/**
 * What differs, in one sentence, before anything else on the page.
 *
 * ADR-025 落地要求: 使用者看到的第一行是任務判定. The two sides are ordered by
 * contract — the run named in the path first, `against` second — so 這一邊 /
 * 另一邊 identify the columns without repeating either uuid.
 */
function ComparisonLead({ data }: { data: RunComparison }) {
  const [left, right] = data.runs;
  const leftVerdict = verdictCell(left);
  const rightVerdict = verdictCell(right);

  return (
    <p className="verdict">
      任務判定：這一邊<strong>{leftVerdict}</strong>，另一邊<strong>{rightVerdict}</strong>
      {leftVerdict === rightVerdict ? "——兩邊相同。" : "——兩邊不同。"}
    </p>
  );
}

/** Column head for a side. The ids are folded once, up beside the heading. */
const SIDE_LABEL = ["這一邊", "另一邊"];

function ComparisonTables({ data }: { data: RunComparison }) {
  const [left, right] = data.runs;
  const sides = [left, right];
  const sharedCostNote = costNote(left) === costNote(right) ? costNote(left) : null;

  return (
    <>
      {/* Checklist 6: this is the page's whole point and it had only a
          <caption>, while the two lesser blocks below carry <h2> — so heading
          navigation skipped the content and landed on the appendices. */}
      <h2>任務判定與執行狀態</h2>
      <div className="table-scroll" tabIndex={0}>
        <table className="compare-table">
          <caption>Run 任務判定與執行狀態對比</caption>
          <thead>
            <tr>
              <th scope="col">項目</th>
              {sides.map((s, i) => (
                <th key={s.run_id} scope="col">
                  {SIDE_LABEL[i]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {/* Two rows, deliberately never one, and the judgement is the first
              of them: 執行成功 ≠ 任務完成, and ADR-025 落地要求 puts the verdict
              on top because that is the question the reader asked. */}
            <tr>
              <th scope="row">任務判定</th>
              {sides.map((s) => (
                <td key={s.run_id}>{verdictCell(s)}</td>
              ))}
            </tr>
            <tr>
              <th scope="row">執行狀態</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {runStatusLabel(s.status)}（<code>{s.status}</code>）
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">Skill 版本</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {/* A uuid twice over, and the diff below answers the question
                      it was standing in for (design §2.6). */}
                  <details>
                    <summary>版本 ID</summary>
                    <code>{s.skill_version_id}</code>
                  </details>
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">最終輸出</th>
              {sides.map((s) => (
                // Untrusted content: inert text, never markup (ADR-001).
                <td key={s.run_id}>{s.final_output ? <pre>{s.final_output}</pre> : "無"}</td>
              ))}
            </tr>
            <tr>
              <th scope="row">錯誤</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.errors && s.errors.length > 0 ? (
                    <ul>
                      {s.errors.map((e, i) => (
                        <li key={`${e.code ?? ""}-${i}`}>
                          [{e.category ?? "?"}/{e.code ?? "?"}] {e.message}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    "無"
                  )}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">延遲</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.duration_ms === undefined ? "未開始執行" : `${s.duration_ms} 毫秒`}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">
                Run 成本（下界）
                {sharedCostNote && <p className="note">{sharedCostNote}</p>}
              </th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {usd(s.cost.usd)}
                  {sharedCostNote ? null : <p className="note">{costNote(s)}</p>}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">
                評估成本
                {/* 每一格都一樣的一句話，而且它講的是這一列與上一列的關係——列首。 */}
                <p className="note">與上一列分開列，不相加。</p>
              </th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.evaluation ? usd(s.evaluation.cost.evaluation_usd) : "未評估"}
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">輸入是否仍在</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  <RerunCell side={s} />
                </td>
              ))}
            </tr>
          </tbody>
        </table>
      </div>

      <h2>逐條驗收條件</h2>
      {data.criterion_matrix.length === 0 ? (
        <p>沒有可對照的驗收條件。</p>
      ) : (
        <div className="table-scroll" tabIndex={0}>
          <table className="compare-table">
            <caption>驗收條件判定矩陣對比</caption>
            <thead>
              <tr>
                <th scope="col">驗收條件</th>
                {sides.map((s, i) => (
                  <th key={s.run_id} scope="col">
                    {SIDE_LABEL[i]}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.criterion_matrix.map((row) => (
                <tr key={row.criterion_id}>
                  <th scope="row">{row.text}</th>
                  {row.results.map((r) => (
                    <td key={r.run_id}>
                      {/* null is "no verdict on this side" — a different fact from
                        undetermined, which is a verdict that was reached. */}
                      {r.result === null ? (
                        <span className="compare-unknown">未評估</span>
                      ) : (
                        CRITERION_LABEL[r.result]
                      )}
                      {r.source === "model" ? <p className="note">模型評估</p> : null}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <h2>Skill 版本差異</h2>
      {data.version_diff_url ? (
        <VersionDiff url={data.version_diff_url} />
      ) : (
        <p>兩次 Run 使用同一個版本，或分屬不同 Skill，沒有版本差異可看。</p>
      )}
    </>
  );
}

/**
 * Whether these inputs could be supplied again, and where to go if they can.
 *
 * The link lands on the preflight screen with the three ids filled in — it does
 * not start anything. `inputs_available: false` drops the link entirely rather
 * than showing a disabled one: the destination would 404 on a deleted test case,
 * and offering a route to it would be offering a re-run the platform cannot do
 * (ADR-003).
 */
function RerunCell({ side }: { side: ComparisonSide }) {
  if (!side.inputs_available) {
    return <>已刪除或已過期，無法以相同輸入重跑；比較內容本身不受影響。</>;
  }
  if (!side.test_case_id) {
    return <>仍在。可用同一個 Test Case 重新試跑，仍須通過執行前權限確認。</>;
  }
  return (
    <>
      仍在。{" "}
      <Link
        to="/lab/run"
        search={{
          skill: side.skill_id,
          version: side.skill_version_id,
          test_case: side.test_case_id,
        }}
      >
        以相同的 Test Case 與版本重新試跑
      </Link>
      {/* §2.4 要的是「這個控制項在／不在的原因」，那是前面的「仍在。」與 inputs_available
          為假時的那一句；連過去之後還要再確認一次是**目的地的事實**，一句話說得完
          （設計 §2.13）。權限摘要本身不在這一頁上，它在那個畫面上逐項可見。 */}
      （會先經過權限確認）
    </>
  );
}

/**
 * One version diff, wherever its address came from.
 *
 * Exported since 2026-08-29 for `SkillDetail`'s 版本歷史 (WS-001 第 4 條): the
 * two callers are the same document — a `files[]` of `FileDiff` — reached by
 * two addresses, one the server hands over in `version_diff_url` and one built
 * from `/skills/{id}/diff` (`api/skills.ts`'s `skillDiffUrl`). A second copy
 * would be a second set of loading and failure states for one answer.
 */
export function VersionDiff({ url }: { url: string }) {
  const diff = useVersionDiff(url);
  if (diff.isPending) return <Loading what="版本差異" />;
  if (diff.error) return <ReadFailure error={diff.error} what="版本差異" />;
  if (diff.data.files.length === 0) return <p>兩個版本的檔案內容相同。</p>;

  return (
    <ul className="file-tree">
      {diff.data.files.map((f) => (
        <li key={f.path}>
          <code>{f.path}</code> · {f.status}
          {f.diff ? (
            <pre className="diff">{f.diff}</pre>
          ) : (
            <p className="note">（二進位或過大，不顯示差異）</p>
          )}
        </li>
      ))}
    </ul>
  );
}
