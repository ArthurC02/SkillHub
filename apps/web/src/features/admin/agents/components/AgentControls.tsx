import { useState } from "react";
import {
  MAX_AGENT_SPEND_CAP_MICROS,
  usd,
  usePlatformAgentBrake,
  usePlatformAgentSpendCap,
  usePlatformAgentSwitch,
  type PlatformAgent,
  type PlatformAgentBrake,
} from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";
import { actionLabel } from "./proposalLabels";

function dollarsToMicros(text: string): number | null {
  if (!/^\d+(\.\d{1,6})?$/.test(text.trim())) return null;
  const micros = Math.round(Number(text) * 1_000_000);
  return micros > 0 && micros <= MAX_AGENT_SPEND_CAP_MICROS ? micros : null;
}

function SpendCapControls({ agent }: { agent: PlatformAgent }) {
  const [dollars, setDollars] = useState(String(agent.daily_spend_cap_usd_micros / 1_000_000));
  const set = usePlatformAgentSpendCap();
  const clear = usePlatformAgentSpendCap();
  const micros = dollarsToMicros(dollars);
  const fieldId = `admin-agent-${agent.name}-cap`;
  return (
    <>
      <ActionForm
        id={`${fieldId}-set`}
        submitLabel={`改 ${agent.name} 的每日上限`}
        pending={set.isPending}
        error={set.error}
        ready={micros !== null}
        contextKey={dollars}
        done={set.isSuccess && "已套用，下一次執行就用這個上限。"}
        onSubmit={(note) =>
          micros !== null &&
          set.mutate({ name: agent.name, daily_spend_cap_usd_micros: micros, note })
        }
      >
        <div className="field">
          <label htmlFor={fieldId}>
            每日花費上限（美元，最多 {usd(MAX_AGENT_SPEND_CAP_MICROS)}）
          </label>
          <input
            id={fieldId}
            inputMode="decimal"
            value={dollars}
            onChange={(event) => {
              setDollars(event.target.value);
              set.reset();
            }}
            readOnly={set.isPending}
            aria-describedby={micros === null ? `${fieldId}-range` : undefined}
          />
          {micros === null && (
            <p id={`${fieldId}-range`} className="note">
              要填大於 0、最多 {usd(MAX_AGENT_SPEND_CAP_MICROS)} 的金額，小數最多六位。
            </p>
          )}
        </div>
      </ActionForm>
      {agent.daily_spend_cap_overridden && (
        <ActionForm
          id={`${fieldId}-clear`}
          submitLabel={`把 ${agent.name} 的上限改回預設 ${usd(agent.default_daily_spend_cap_usd_micros)}`}
          pending={clear.isPending}
          error={clear.error}
          done={clear.isSuccess && "已改回預設。"}
          onSubmit={(note) =>
            clear.mutate({ name: agent.name, daily_spend_cap_usd_micros: null, note })
          }
        />
      )}
    </>
  );
}

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
        每日花費上限 {usd(agent.daily_spend_cap_usd_micros)}
        {agent.daily_spend_cap_overridden
          ? `（營運者設定；預設 ${usd(agent.default_daily_spend_cap_usd_micros)}）`
          : "（預設）"}
        ；可讀：{agent.tools.join("、") || "無"}
      </p>
      <p className="note">可提案：{agent.actions.map(actionLabel).join("、") || "無"}</p>
      <p className="note">模型角色：{agent.model_role}</p>
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
      <SpendCapControls agent={agent} />
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
