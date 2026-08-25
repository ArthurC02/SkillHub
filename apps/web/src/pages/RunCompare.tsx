import { Loading } from "../components/Loading";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useRunComparison, useVersionDiff } from "../api/evaluation";
import type { ComparisonSide, RunComparison } from "../api/evaluation";
import { CRITERION_LABEL, OVERALL_LABEL, RUN_STATUS_LABEL } from "./RunEvaluation";

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

  const pickForm = (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        // The selection lives in the URL so a comparison is linkable, the same
        // rule the Explorer's compare screen follows.
        void navigate({
          to: "/runs/$runId/compare",
          params: { runId },
          search: { against: draft },
        });
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
    </form>
  );

  return (
    <section className="page">
      <h1>Run 比較</h1>

      {/* Checklist 1: the page led with a uuid, then a form, then two columns
          headed by 36-char ids, and no sentence anywhere said what differed.
          Both verdicts were already computed; now they are the headline. */}
      {comparison.data && <ComparisonLead data={comparison.data} />}
      <p className="note">比較只是讀取，不會改動任何一個 Run 的歷史資料。</p>

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
      {comparison.data ? (
        <details>
          <summary>換一個要比較的 Run</summary>
          {pickForm}
        </details>
      ) : (
        pickForm
      )}

      {against === "" && <p>輸入另一個 Run 的 ID 後開始比較。</p>}
      {comparison.isPending && against !== "" && <Loading what="比較" />}
      {comparison.error && <p role="alert">無法比較：{comparison.error.message}</p>}
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
                  {RUN_STATUS_LABEL[s.status] ?? s.status}（<code>{s.status}</code>）
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
              <th scope="row">Run 成本（下界）</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {usd(s.cost.usd)}
                  <p className="note">
                    {s.cost.is_lower_bound ? "這是下界，不是總額。" : ""}
                    權威來源：{s.cost.authoritative_source}
                  </p>
                </td>
              ))}
            </tr>
            <tr>
              <th scope="row">評估成本</th>
              {sides.map((s) => (
                <td key={s.run_id}>
                  {s.evaluation ? usd(s.evaluation.cost.evaluation_usd) : "未評估"}
                  <p className="note">與上一列分開列，不相加。</p>
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
                        <span className="compare-unknown">無判定</span>
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
      ：連過去的是執行前權限確認畫面，仍須在那裡確認一次才會開始 Run。
    </>
  );
}

function VersionDiff({ url }: { url: string }) {
  const diff = useVersionDiff(url);
  if (diff.isPending) return <Loading what="版本差異" />;
  if (diff.error) return <p role="alert">無法讀取版本差異：{diff.error.message}</p>;
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
