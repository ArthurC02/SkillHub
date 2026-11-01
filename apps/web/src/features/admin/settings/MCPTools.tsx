import { useState } from "react";
import {
  MCPToolDraftNoteMaxLength as MAX_NOTE,
  MCPToolDraftParametersMaxLength as MAX_PARAMETERS,
  MCPToolDraftPurposeMaxLength as MAX_PURPOSE,
  MCPToolDraftServerMaxLength as MAX_SERVER,
  MCPToolDraftServerUrlMaxLength as MAX_URL,
  MCPToolDraftToolMaxLength as MAX_TOOL,
} from "@skillhub/api-client-ts/limits";
import {
  useAddMCPTool,
  useMCPTools,
  useRetireMCPTool,
  type MCPTool,
  type MCPToolDraft,
} from "../admin.service";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { Timestamp } from "../../../shared/ui/Timestamp";
import { ActionForm } from "../components/ActionForm";

const SERVER_FORM = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const TOOL_FORM = /^[A-Za-z0-9]+([_-][A-Za-z0-9]+)*$/;
const URL_FORM = /^https:\/\/[^\s/?#]+/;

type Fields = Omit<MCPToolDraft, "note">;
const EMPTY: Fields = { server: "", tool: "", purpose: "", parameters: "", server_url: "" };

function fits(value: string, max: number, form?: RegExp) {
  const text = value.trim();
  return text !== "" && [...text].length <= max && (!form || form.test(text));
}

function complete(f: Fields) {
  return (
    fits(f.server, MAX_SERVER, SERVER_FORM) &&
    fits(f.tool, MAX_TOOL, TOOL_FORM) &&
    fits(f.purpose, MAX_PURPOSE) &&
    [...f.parameters.trim()].length <= MAX_PARAMETERS &&
    fits(f.server_url, MAX_URL, URL_FORM)
  );
}

const FIELD_LABELS: { key: keyof Fields; label: string; max: number }[] = [
  { key: "server", label: "伺服器名稱（小寫英數，以 - 連接）", max: MAX_SERVER },
  { key: "tool", label: "工具名稱（英數，以 _ 或 - 連接）", max: MAX_TOOL },
  { key: "purpose", label: "用途", max: MAX_PURPOSE },
  { key: "parameters", label: "參數摘要（可留空）", max: MAX_PARAMETERS },
  { key: "server_url", label: "伺服器位址（https://）", max: MAX_URL },
];

function AddTool() {
  const add = useAddMCPTool();
  const [fields, setFields] = useState(EMPTY);
  const ready = complete(fields);
  return (
    <details>
      <summary>新增工具</summary>
      <ActionForm
        id="admin-mcp-tools-add"
        submitLabel="加入清單"
        pending={add.isPending}
        error={add.error}
        ready={ready}
        contextKey={JSON.stringify(fields)}
        done={add.isSuccess && "已加入清單，下一次創作就能選用。"}
        onSubmit={(note) => ready && add.mutate({ ...fields, note: note.slice(0, MAX_NOTE) })}
      >
        {FIELD_LABELS.map(({ key, label, max }) => (
          <div className="field" key={key}>
            <label htmlFor={`admin-mcp-tools-${key}`}>{label}</label>
            <input
              id={`admin-mcp-tools-${key}`}
              value={fields[key]}
              maxLength={max}
              onChange={(event) => {
                setFields({ ...fields, [key]: event.target.value });
                add.reset();
              }}
              readOnly={add.isPending}
            />
          </div>
        ))}
      </ActionForm>
    </details>
  );
}

function RetireTool({ tool }: { tool: MCPTool }) {
  const retire = useRetireMCPTool();
  return (
    <details>
      <summary>退役 {tool.reference}</summary>
      <ActionForm
        id={`admin-mcp-tools-retire-${tool.id}`}
        submitLabel="退役這個工具"
        tone="caution"
        pending={retire.isPending}
        error={retire.error}
        onSubmit={(note) => retire.mutate({ id: tool.id, note })}
      />
    </details>
  );
}

function ToolEntry({ tool }: { tool: MCPTool }) {
  return (
    <li>
      <p className="badge-row">
        <code>{tool.reference}</code>
        {tool.retired_at && <span className="badge">已退役</span>}
      </p>
      <p>{tool.purpose}</p>
      <p>參數：{tool.parameters || "無"}</p>
      <p>位址：{tool.server_url}</p>
      <p>
        加入理由：{tool.reason}（<Timestamp at={tool.created_at} />）
      </p>
      {tool.retired_at ? (
        <p>
          退役理由：{tool.retire_reason}（<Timestamp at={tool.retired_at} />）
        </p>
      ) : (
        <RetireTool tool={tool} />
      )}
    </li>
  );
}

export function MCPTools() {
  const tools = useMCPTools();
  return (
    <section className="admin-settings-policy" aria-labelledby="admin-mcp-tools-heading">
      <h2 id="admin-mcp-tools-heading" tabIndex={-1}>
        Skill 可宣告的 MCP 工具
      </h2>
      <p className="note">
        Skill 在 allowed-tools 以 mcp__伺服器__工具
        引用這裡的工具。創作只能選用清單上的工具；匯入的套件引用清單外的名稱會收到警告。試跑不會連到這些伺服器。
      </p>
      {tools.isPending && <Loading what="MCP 工具清單" />}
      <ReadFailure error={tools.error} what="MCP 工具清單">
        <p role="alert">暫時無法讀取 MCP 工具清單，請再試一次。</p>
        <button type="button" disabled={tools.isFetching} onClick={() => void tools.refetch()}>
          再讀取 MCP 工具清單
        </button>
      </ReadFailure>
      {tools.data &&
        (tools.data.length === 0 ? (
          <p>清單是空的：創作出的 Skill 目前不能宣告任何 MCP 工具。</p>
        ) : (
          <ul>
            {tools.data.map((tool) => (
              <ToolEntry key={tool.id} tool={tool} />
            ))}
          </ul>
        ))}
      {tools.data && <AddTool />}
    </section>
  );
}
