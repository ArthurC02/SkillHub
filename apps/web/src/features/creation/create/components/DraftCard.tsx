import { Link } from "@tanstack/react-router";
import type { CreationSnapshot, CreationState } from "../../creation.service";
import { runStatusLabel, type RunListItem } from "../../../runs";
import { Reveal } from "../../../../shared/ui/Reveal";
import { findRunObservation } from "../create.model";
import type { Perform } from "../create.commands";
import { DraftFindings } from "./DraftFindings";

export function DraftCard({
  p,
  draft,
  state,
  terminal,
  latest,
  locked,
  perform,
}: {
  p: CreationSnapshot;
  draft: NonNullable<CreationSnapshot["draft"]>;
  state: CreationState;
  terminal: boolean;
  latest: RunListItem | undefined;
  locked: boolean;
  perform: Perform;
}) {
  const run = p.candidate?.run_id ? findRunObservation(p.messages, p.candidate.run_id) : undefined;
  const runNotPassing =
    !!run && (run.execution_status !== "succeeded" || run.evaluation?.overall !== "met");
  return (
    <section>
      <header className="card-header">
        <h4>Skill 草稿：{draft.skill.name}</h4>
        <span className="card-tag" data-tone={draft.blocked ? "danger" : "done"}>
          {draft.blocked ? "靜態檢查阻擋保存" : "已完成靜態檢查"}
        </span>
      </header>
      <p>{draft.skill.description}</p>
      <p>
        允許工具：{draft.skill.allowed_tools || "未宣告"}。相容條件：
        {draft.skill.compatibility || "未宣告"}。
      </p>
      {p.previous_draft && (
        <details>
          <summary>比較上一份草稿（revision {p.previous_draft.revision}）</summary>
          <pre className="skill-md">
            <Reveal text={p.previous_draft.skill.body} />
          </pre>
          {p.previous_draft.skill.files?.map((f) => (
            <pre key={f.path}>{f.path + "\n" + f.content}</pre>
          ))}
        </details>
      )}
      <pre className="skill-md">
        <Reveal text={draft.skill.body} />
      </pre>
      {draft.skill.files?.map((f) => (
        <details key={f.path}>
          <summary>{f.path}</summary>
          <pre className="skill-md">
            <Reveal text={f.content} />
          </pre>
        </details>
      ))}
      <DraftFindings raw={draft.validation} />
      <p className="note">
        {draft.blocked ? "請補充需求後修訂。" : "靜態檢查通過不代表試跑成功。"}
      </p>
      {p.candidate && (
        <>
          {p.adopted && <p>已直接採用現有 Skill；這個候選版本是它的複本，沒有生成任何內容。</p>}
          <p>
            <Link
              to="/lab/run"
              search={{
                skill: p.candidate.skill_id,
                version: p.candidate.version_id,
                test_case: p.candidate.test_case_id,
              }}
            >
              檢查權限與費用後試跑此版本
            </Link>
          </p>
          {p.candidate.test_case_id && <p>已依確認的驗收條件建立 Test Case</p>}
          {p.candidate.run_id ? (
            <>
              <Link to="/runs/$runId" params={{ runId: p.candidate.run_id }}>
                查看這次 Run 結果
              </Link>
              {run && (
                <p>
                  試跑結果：{run.execution_status}；評估：
                  {run.evaluation?.overall ?? "無評估"}
                </p>
              )}
            </>
          ) : (
            <p>目前尚未連結試跑結果。</p>
          )}
          {!terminal &&
            (latest ? (
              latest.run_id === p.candidate.run_id ? (
                <p>最新試跑結果已帶回會話；模型的建議在對話裡。</p>
              ) : (
                <>
                  <p>
                    最新試跑：{runStatusLabel(latest.status)}；評估：
                    {latest.evaluation.label}
                  </p>
                  <button
                    disabled={locked}
                    onClick={() => void perform("attach_run", { run_id: latest.run_id })}
                  >
                    把最新試跑結果帶回來改善
                  </button>
                </>
              )
            ) : (
              <p>試跑完成後，這裡會出現「把最新試跑結果帶回來改善」。</p>
            ))}
        </>
      )}
      {!terminal && (
        <>
          <p>
            保存將採用目前顯示的草稿與版本。
            {!p.candidate?.run_id && "這份草稿尚未試跑。"}
            {runNotPassing && "試跑未通過或未評估；保存前請確認。"}
          </p>
          <div className="card-actions">
            {!p.candidate && p.pending_action !== "confirm_duplicate" && (
              <button
                disabled={locked || draft.blocked || !draft.content_hash}
                onClick={() => void perform("materialize", { content_hash: draft.content_hash })}
              >
                建立私人候選版本
              </button>
            )}
            <button
              className="action"
              disabled={locked || draft.blocked || !draft.content_hash}
              onClick={() => void perform("finalize", { content_hash: draft.content_hash })}
            >
              確認保存到私人工作區
            </button>
          </div>
        </>
      )}
      {state === "saved" && p.candidate && (
        <Link to="/skills/$skillId" params={{ skillId: p.candidate.skill_id }}>
          開啟已保存的 Skill
        </Link>
      )}
    </section>
  );
}
