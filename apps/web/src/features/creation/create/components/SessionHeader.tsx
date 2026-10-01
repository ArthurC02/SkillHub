import { useState } from "react";
import { Link } from "@tanstack/react-router";
import type { CreationLimits, CreationSession, CreationSnapshot } from "../../creation.service";
import { creationStateLabel } from "../../creation.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { nextStepBudget, points, raiseBudgetProblem } from "../create.model";
import type { Perform } from "../create.commands";
import { AgentAvatar } from "./ConversationLog";

function NextStep({
  costCredits,
  remainingCredits,
  roomForAnother,
}: {
  costCredits: number;
  remainingCredits: number;
  roomForAnother: boolean;
}) {
  if (roomForAnother) {
    return (
      <span className="creation-next-step">
        {" "}
        · 下一步最多 {points(costCredits)}，預算還有 {points(remainingCredits)}
      </span>
    );
  }
  return (
    <span className="creation-next-step">
      {" "}
      ·{" "}
      <strong>
        預算只剩 {points(remainingCredits)}，不夠再走一步的 {points(costCredits)}
      </strong>
      ，展開可以提高預算
    </span>
  );
}

export function SessionHeader({
  session,
  p,
  limits,
  terminal,
  working,
  busy,
  perform,
  onError,
  sessionCount,
  sessionsOpen,
  onToggleSessions,
}: {
  session: CreationSession | undefined;
  p: CreationSnapshot | undefined;
  limits: CreationLimits | undefined;
  terminal: boolean;
  working: boolean;
  busy: boolean;
  perform: Perform;
  onError: (error: Error) => void;
  sessionCount: number | undefined;
  sessionsOpen: boolean;
  onToggleSessions: () => void;
}) {
  const [raiseBudget, setRaiseBudget] = useState("");
  const submitRaiseBudget = async () => {
    if (!p || !limits) return;
    const problem = raiseBudgetProblem(raiseBudget, p.budget_credits, limits.max_budget_credits);
    if (problem) {
      onError(new Error(problem));
      return;
    }
    await perform("raise_budget", { budget_credits: Number(raiseBudget) });
  };
  return (
    <header className="creation-bar">
      <nav aria-label="離開這一頁">
        <Link to="/library" className="bar-back" aria-label="回到資產庫">
          ←
        </Link>
      </nav>
      <AgentAvatar />
      <div className="bar-title">
        <h1>和 Agent 一起創作小工具</h1>
        <span className="creation-state">
          {session ? (
            <>
              <span role="status">{creationStateLabel(session.state)}</span>
              {p && limits && ` · ${p.steps}／${limits.max_steps} 步`}
            </>
          ) : (
            "說出任務，一步步做成你的小工具"
          )}
        </span>
      </div>
      {session && p && (
        <SessionCostDetails
          session={session}
          p={p}
          limits={limits}
          terminal={terminal}
          working={working}
          busy={busy}
          perform={perform}
          raiseBudget={raiseBudget}
          onRaiseBudget={setRaiseBudget}
          onSubmitRaiseBudget={submitRaiseBudget}
        />
      )}
      <button
        type="button"
        className="creation-session-toggle"
        aria-controls="creation-sessions"
        aria-expanded={sessionsOpen}
        onClick={onToggleSessions}
      >
        創作清單{sessionCount === undefined ? "" : ` · ${sessionCount}`}
      </button>
    </header>
  );
}

function SessionCostDetails({
  session,
  p,
  limits,
  terminal,
  working,
  busy,
  perform,
  raiseBudget,
  onRaiseBudget,
  onSubmitRaiseBudget,
}: {
  session: CreationSession;
  p: CreationSnapshot;
  limits: CreationLimits | undefined;
  terminal: boolean;
  working: boolean;
  busy: boolean;
  perform: Perform;
  raiseBudget: string;
  onRaiseBudget: (value: string) => void;
  onSubmitRaiseBudget: () => void;
}) {
  return (
    <details className="creation-details">
      <summary>
        費用 {p.spent_credits === undefined || p.usage_unknown ? "未知" : p.spent_credits} /{" "}
        {points(p.budget_credits)}
        {limits && !terminal && <NextStep {...nextStepBudget(p, limits.min_budget_credits)} />}
      </summary>
      <div>
        <p className="note">
          仍占用預算 {p.reserved_credits} 點
          {limits && (
            <>
              {" "}
              · 工具 {p.tool_calls}／{limits.max_tool_calls} 次
            </>
          )}
        </p>
        {!terminal && (
          <p className="note">
            可進行到 <Timestamp at={session.deadline} /> · 紀錄保留到{" "}
            <Timestamp at={session.expires_at} />
          </p>
        )}
        {limits && (
          <>
            <label>
              提高這次預算上限（點）
              <input
                aria-label="提高這次預算上限（點）"
                inputMode="decimal"
                disabled={busy}
                value={raiseBudget}
                onChange={(e) => onRaiseBudget(e.target.value)}
              />
            </label>
            <button type="button" disabled={busy} onClick={() => void onSubmitRaiseBudget()}>
              提高預算後繼續
            </button>
          </>
        )}
        {!terminal && !working && session.state !== "failed" && (
          <button
            type="button"
            className="destructive"
            disabled={busy}
            onClick={() => void perform("cancel")}
          >
            取消這次創作
          </button>
        )}
      </div>
    </details>
  );
}
