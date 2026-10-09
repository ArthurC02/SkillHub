import {
  usd,
  usePlatformAgentBrake,
  usePlatformAgentSwitch,
  type PlatformAgent,
  type PlatformAgentBrake,
} from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";
import { actionLabel } from "./proposalLabels";
import "./AgentControls.css";

function AgentRow({ agent }: { agent: PlatformAgent }) {
  const toggle = usePlatformAgentSwitch();
  const next = !agent.enabled;
  return (
    <li className="download-item">
      <p>
        <strong>{agent.name}</strong>
      </p>
      <p className="badge-row">
        <span className={agent.enabled ? "badge" : "badge badge-danger"}>
          {agent.enabled ? "啟用中" : "停用中"}
        </span>
      </p>
      <p>{agent.purpose}</p>
      <p className="note">
        每日花費上限 {usd(agent.daily_spend_cap_usd_micros)}；可讀：{agent.tools.join("、") || "無"}
      </p>
      <p className="note">可提案：{agent.actions.map(actionLabel).join("、") || "無"}</p>
      <p className="note">模型角色：{agent.model_role}</p>
      <p className="note">
        負責營運者：
        {agent.owner_user_id ? (
          <code className="agent-owner-id">{agent.owner_user_id}</code>
        ) : (
          "未記錄"
        )}
      </p>
      <ActionForm
        id={`admin-agent-${agent.name}`}
        submitLabel={next ? `啟用 ${agent.name}` : `停用 ${agent.name}`}
        tone={next ? undefined : "caution"}
        pending={toggle.isPending}
        error={toggle.error}
        done={
          toggle.isSuccess &&
          (toggle.variables.enabled
            ? "已啟用，下一次排程會執行。"
            : "已停用，執行中的那一次會在下一步之前停下。")
        }
        onSubmit={(note) => toggle.mutate({ name: agent.name, enabled: next, note })}
      />
    </li>
  );
}

export function BrakeControls({ brake }: { brake?: PlatformAgentBrake }) {
  const engage = usePlatformAgentBrake("PUT");
  const release = usePlatformAgentBrake("DELETE");
  return (
    <>
      <p>
        <span className={brake ? "badge badge-danger" : "badge"}>
          {brake ? "煞車拉下：所有 Agent 停止" : "煞車放開"}
        </span>
      </p>
      {brake && (
        <>
          <p>理由：{brake.reason}</p>
          <p>
            拉下於 <Timestamp at={brake.engaged_at} />
          </p>
        </>
      )}
      <div
        onInput={() => {
          engage.reset();
          release.reset();
        }}
      >
        {brake ? (
          <ActionForm
            id="admin-agent-brake-release"
            submitLabel="放開 Agent 煞車"
            pending={release.isPending}
            error={release.error}
            onSubmit={(note) => release.mutate({ note })}
          />
        ) : (
          <ActionForm
            id="admin-agent-brake-engage"
            submitLabel="拉下 Agent 煞車"
            tone="caution"
            pending={engage.isPending}
            error={engage.error}
            onSubmit={(note) => engage.mutate({ note })}
          />
        )}
        {brake && engage.isSuccess && (
          <p className="notice notice-success" role="status">
            已拉下，所有 Agent 在下一步之前停下。
          </p>
        )}
        {!brake && release.isSuccess && (
          <p className="notice notice-success" role="status">
            已放開，啟用中的 Agent 下一次排程會執行。
          </p>
        )}
      </div>
    </>
  );
}

export function AgentControls({ agents }: { agents: PlatformAgent[] }) {
  return (
    <>
      <ul className="download-list">
        {agents.map((agent) => (
          <AgentRow agent={agent} key={agent.name} />
        ))}
      </ul>
    </>
  );
}
