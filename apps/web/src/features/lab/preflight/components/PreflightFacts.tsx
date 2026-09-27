import { Timestamp } from "../../../../shared/ui/Timestamp";
import { bytes } from "../../../../shared/format";
import type { CostEstimate, PreflightSummary, RunQuota } from "../../lab.service";
import { ceiling, limit, count, seconds, tokens, SCRIPT_LABEL } from "../preflight.model";

const isolationLabels: Record<string, string> = {
  strong: "強（獨立核心）",
  weak: "弱（與主機共用核心）",
  none: "無（沒有隔離邊界）",
};

function isolationText(strength: string) {
  return isolationLabels[strength] ?? strength;
}

export function PreflightFacts({
  summary,
  cost,
  quota,
}: {
  summary: PreflightSummary;
  cost: CostEstimate | undefined;
  quota: RunQuota | undefined;
}) {
  return (
    <dl data-role="evidence">
      <dt>預估點數（估計值）</dt>
      <dd>
        {cost ? (
          <>
            {cost.low_credits} – {cost.high_credits} 點（常見約 {cost.typical_credits} 點）
            <p>{cost.basis}</p>
          </>
        ) : (
          <>未測量——這個伺服器版本沒有回報預估點數，不代表這次 Run 不用點。</>
        )}
      </dd>
      {quota && (
        <>
          <dt>剩餘試跑額度</dt>
          <dd>
            今天還可以跑 {quota.remaining_today} 次、這個週期還可以跑 {quota.remaining_window} 次。
            額度下一次增加不會早於 <Timestamp at={quota.window_resets_at} />。
            <p className="note">
              上限：每日 {quota.limits.daily} 次、每 {quota.limits.window_days} 天{" "}
              {quota.limits.window} 次、同時進行 {quota.limits.concurrent} 個。 這些數字就是建立 Run
              時擋你的那一份計數，不是另外顯示的估計。
            </p>
          </dd>
        </>
      )}

      <dt>Dataset</dt>
      <dd>
        {summary.datasets.length === 0 ? (
          "無"
        ) : (
          <ul>
            {summary.datasets.map((d) => (
              <li key={d.dataset_id}>
                {d.file_name}（{bytes(d.size_bytes)}）
              </li>
            ))}
          </ul>
        )}
        {summary.datasets.length > 0 && <p>合計 {bytes(summary.dataset_total_bytes)}</p>}
      </dd>

      <dt>Script</dt>
      <dd>
        {SCRIPT_LABEL[summary.scripts.status]}
        {summary.scripts.findings.length > 0 && (
          <ul>
            {summary.scripts.findings.map((f) => (
              <li key={f}>{f}</li>
            ))}
          </ul>
        )}
      </dd>

      <dt>工具</dt>
      <dd>{summary.tools.length === 0 ? "無（沒有授予任何工具）" : summary.tools.join("、")}</dd>

      <dt>MCP Server</dt>
      <dd>{summary.mcp_servers.length === 0 ? "無" : summary.mcp_servers.join("、")}</dd>

      <dt>網路</dt>
      <dd>
        {summary.network.mode}
        {summary.network.allow.length === 0
          ? "（允許清單為空:不能連出任何位址）"
          : `（允許 ${summary.network.allow.join("、")}）`}
      </dd>

      <dt>Secrets</dt>
      <dd>
        {summary.injected_secrets.length === 0 ? (
          "無（不會注入任何 Secret）"
        ) : (
          <>
            {summary.injected_secrets.join("、")}
            <p>只顯示名稱;實際值為每個 Run 專屬的短效憑證,不會顯示於任何畫面。</p>
          </>
        )}
      </dd>

      <dt>Provider</dt>
      <dd>
        {summary.provider.name}
        {summary.provider.isolation_strength &&
          `（隔離:${isolationText(summary.provider.isolation_strength)}）`}
      </dd>

      <dt>資源上限</dt>
      <dd>
        vCPU {limit(summary.resource_limits.vcpu, count)}、記憶體{" "}
        {ceiling(summary.resource_limits.memory_bytes)}、 磁碟{" "}
        {ceiling(summary.resource_limits.disk_bytes)}、 時間上限{" "}
        {limit(summary.resource_limits.wall_clock_hard_seconds, seconds)}、 Token{" "}
        {limit(summary.resource_limits.token_budget?.max_input_tokens, tokens)} 進 /{" "}
        {limit(summary.resource_limits.token_budget?.max_output_tokens, tokens)} 出
        <p className="note" data-role="teaching">
          Token 上限能跑幾輪，取決於每一輪的工具呼叫次數——每次工具結果回填都要重送整個前綴，
          所以同樣的 300K input，工具密集的 Run 大約只夠 5 輪，純對話大約夠 15 輪。
        </p>
      </dd>

      <dt>進階限制與 Provider 細節</dt>
      <dd>
        <details>
          <summary>展開其餘一併確認的欄位</summary>
          <ul>
            <li>行程數上限：{limit(summary.resource_limits.max_pids, count)}</li>
            <li>開檔數上限：{limit(summary.resource_limits.max_open_files, count)}</li>
            <li>
              產出檔案總量上限：{ceiling(summary.resource_limits.artifact_total_bytes)}、單檔{" "}
              {ceiling(summary.resource_limits.artifact_file_bytes)}
            </li>
            <li>
              軟性時間上限：{limit(summary.resource_limits.wall_clock_soft_seconds, seconds)}
              （先要求收尾; 硬性上限{" "}
              {limit(summary.resource_limits.wall_clock_hard_seconds, seconds)}才是強制中止）
            </li>
            <li>Provider 是否 rootless：{summary.provider.rootless ? "是" : "否"}</li>
            <li>Runtime：{summary.provider.runtime ?? "未測量"}</li>
            <li>Runtime 版本：{summary.provider.runtime_version ?? "未測量"}</li>
          </ul>
        </details>
      </dd>
    </dl>
  );
}
