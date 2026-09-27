import { Fragment } from "react";
import type { CreationSession } from "../../creation.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { stepDescription } from "../create.model";
import type { Perform } from "../create.commands";
import { ModelMarkdown } from "./ModelMarkdown";
import { Attachments } from "./Attachments";
import { ToolObservation } from "./ToolObservation";

export function AgentAvatar() {
  return (
    <span className="agent-avatar" aria-hidden="true">
      ✦
    </span>
  );
}

export function ConversationLog({
  session,
  thumbs,
  working,
  busy,
  perform,
}: {
  session: CreationSession;
  thumbs: Map<string, string>;
  working: boolean;
  busy: boolean;
  perform: Perform;
}) {
  const p = session.snapshot;
  const lastSaid = [...p.messages].reverse().find((m) => m.role === "user")?.content ?? "";
  return (
    <div role="log" aria-label="與 Agent 的對話">
      <ol className="creation-log">
        {p.messages.map((m, i) => {
          const here = (p.attachments ?? []).filter((a) => a.message_index === i);
          return (
            <Fragment key={i}>
              {m.role !== "user" && here.length > 0 && (
                <li data-role="user">
                  <span className="creation-who">你</span>
                  <Attachments list={here} thumbs={thumbs} />
                </li>
              )}
              <li data-role={m.role} data-index={i}>
                {m.role === "assistant" && <AgentAvatar />}
                <span className="creation-who">
                  {{ user: "你", assistant: "Agent", tool: "工具結果" }[m.role]}
                </span>
                {m.created_at && <Timestamp at={m.created_at} />}
                {m.role === "tool" ? (
                  <ToolObservation raw={m.content} />
                ) : m.role === "assistant" ? (
                  <ModelMarkdown text={m.content} />
                ) : (
                  <span className="creation-text">{m.content}</span>
                )}
                {m.role === "user" && here.length > 0 && (
                  <Attachments list={here} thumbs={thumbs} />
                )}
                {session.state === "failed" &&
                  m.role === "assistant" &&
                  i === p.messages.length - 1 && (
                    <div className="turn-actions">
                      <button
                        type="button"
                        disabled={busy || !lastSaid}
                        onClick={() => void perform("message", { message: lastSaid })}
                      >
                        再送一次上一句
                      </button>
                      <button
                        type="button"
                        className="destructive"
                        disabled={busy}
                        onClick={() => void perform("cancel")}
                      >
                        取消這次創作
                      </button>
                    </div>
                  )}
              </li>
            </Fragment>
          );
        })}
        {(p.attachments ?? []).some((a) => a.message_index >= p.messages.length) && (
          <li data-role="user">
            <span className="creation-who">你</span>
            <Attachments
              list={(p.attachments ?? []).filter((a) => a.message_index >= p.messages.length)}
              thumbs={thumbs}
            />
          </li>
        )}
        {working && (
          <li data-role="assistant" data-pending="">
            <AgentAvatar />
            <span className="creation-who">Agent</span>
            <span className="typing" aria-hidden="true">
              <span />
              <span />
              <span />
            </span>
            <span className="creation-text">{stepDescription(p)}</span>
            <p className="note">
              這一步會自己結束。可以關掉這一頁，回來時從「對話紀錄」繼續；上次更新{" "}
              <Timestamp at={session.updated_at} relative />
            </p>
            <div className="turn-actions">
              <button type="button" disabled={busy} onClick={() => void perform("stop_step")}>
                停止這一步
              </button>
              <button
                type="button"
                className="destructive"
                disabled={busy}
                onClick={() => void perform("cancel")}
              >
                取消這次創作
              </button>
            </div>
          </li>
        )}
      </ol>
    </div>
  );
}
