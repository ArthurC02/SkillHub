import { useEffect, useRef } from "react";
import {
  usd,
  usePlatformAgentSwitch,
  type usePlatformAgentBrake,
  type PlatformAgent,
  type PlatformAgentBrake,
} from "../../admin.service";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ActionForm } from "../../components/ActionForm";
import { actionLabel } from "./proposalLabels";
import "./AgentControls.css";

function switchResult(name: string, enabled: boolean, unverified: boolean) {
  if (unverified) return `已送出${enabled ? "啟用" : "停用"} ${name}；最新狀態尚未確認。`;
  return enabled ? "已啟用，下一次排程會執行。" : "已停用，執行中的那一次會在下一步之前停下。";
}

function AgentRow({
  agent,
  readable,
  fetching,
  refresh,
}: {
  agent: PlatformAgent;
  readable: boolean;
  fetching: boolean;
  refresh: () => void;
}) {
  const toggle = usePlatformAgentSwitch();
  const next = !agent.enabled;
  const unverified = toggle.isSuccess && (!readable || agent.enabled !== toggle.variables.enabled);
  const result = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (toggle.isSuccess) {
      result.current?.focus();
      result.current?.scrollIntoView?.({ block: "center" });
    }
  }, [toggle.isSuccess]);
  if (!readable && !toggle.isSuccess) return null;
  return (
    <li className="download-item" onInput={() => toggle.reset()}>
      <p>
        <strong>{agent.name}</strong>
      </p>
      {readable && !unverified && (
        <>
          <p className="badge-row">
            <span className={agent.enabled ? "badge" : "badge badge-danger"}>
              {agent.enabled ? "啟用中" : "停用中"}
            </span>
          </p>
          <p>{agent.purpose}</p>
          <p className="note">
            每日花費上限 {usd(agent.daily_spend_cap_usd_micros)}；可讀：
            {agent.tools.join("、") || "無"}
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
            onSubmit={(note) => toggle.mutate({ name: agent.name, enabled: next, note })}
          />
        </>
      )}
      {toggle.isSuccess && (
        <p
          id={`admin-agent-${agent.name}-result`}
          ref={result}
          tabIndex={-1}
          className={unverified ? "notice notice-warning" : "notice notice-success"}
          role="status"
        >
          {switchResult(agent.name, toggle.variables.enabled, unverified)}
        </p>
      )}
      {unverified && (
        <button type="button" disabled={fetching} onClick={refresh}>
          {fetching ? "重新整理中…" : "重新整理此 Agent"}
        </button>
      )}
    </li>
  );
}

export function BrakeControls({
  brake,
  engage,
  release,
  unverified,
  readable,
  fetching,
  refresh,
}: {
  brake?: PlatformAgentBrake;
  engage: ReturnType<typeof usePlatformAgentBrake>;
  release: ReturnType<typeof usePlatformAgentBrake>;
  unverified: boolean;
  readable: boolean;
  fetching: boolean;
  refresh: () => void;
}) {
  const result = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (engage.isSuccess || release.isSuccess) {
      result.current?.focus();
      result.current?.scrollIntoView?.({ block: "center" });
    }
  }, [engage.isSuccess, release.isSuccess]);
  const sentence = engage.isSuccess
    ? unverified
      ? "已送出拉下煞車；最新狀態尚未確認。"
      : "已拉下，所有 Agent 在下一步之前停下。"
    : release.isSuccess
      ? unverified
        ? "已送出放開煞車；最新狀態尚未確認。"
        : "已放開，啟用中的 Agent 下一次排程會執行。"
      : undefined;
  return (
    <>
      <div
        onInput={() => {
          engage.reset();
          release.reset();
        }}
      >
        {readable && !unverified && (
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
          </>
        )}
        {sentence && (
          <p
            id="admin-agent-brake-result"
            ref={result}
            tabIndex={-1}
            className={unverified ? "notice notice-warning" : "notice notice-success"}
            role="status"
          >
            {sentence}
          </p>
        )}
        {(!readable || unverified) && (
          <button type="button" disabled={fetching} onClick={refresh}>
            {fetching ? "重新整理中…" : "重新整理 Agent 狀態"}
          </button>
        )}
      </div>
    </>
  );
}

export function AgentControls({
  agents,
  readable,
  fetching,
  refresh,
}: {
  agents: PlatformAgent[];
  readable: boolean;
  fetching: boolean;
  refresh: () => void;
}) {
  return (
    <>
      <ul className="download-list">
        {agents.map((agent) => (
          <AgentRow
            agent={agent}
            key={agent.name}
            readable={readable}
            fetching={fetching}
            refresh={refresh}
          />
        ))}
      </ul>
    </>
  );
}
