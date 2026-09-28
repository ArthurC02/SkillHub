import type { PreflightSummary } from "../../lab.service";
import { ceiling, limit, count, seconds, tokens } from "../preflight.model";

export function ResourceLimitsFacts({
  resourceLimits,
  provider,
}: {
  resourceLimits: PreflightSummary["resource_limits"];
  provider: PreflightSummary["provider"];
}) {
  return (
    <>
      <dt>資源上限</dt>
      <dd>
        vCPU {limit(resourceLimits.vcpu, count)}、記憶體 {ceiling(resourceLimits.memory_bytes)}、
        磁碟 {ceiling(resourceLimits.disk_bytes)}、 時間上限{" "}
        {limit(resourceLimits.wall_clock_hard_seconds, seconds)}、 Token{" "}
        {limit(resourceLimits.token_budget?.max_input_tokens, tokens)} 進 /{" "}
        {limit(resourceLimits.token_budget?.max_output_tokens, tokens)} 出
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
            <li>行程數上限：{limit(resourceLimits.max_pids, count)}</li>
            <li>開檔數上限：{limit(resourceLimits.max_open_files, count)}</li>
            <li>
              產出檔案總量上限：{ceiling(resourceLimits.artifact_total_bytes)}、單檔{" "}
              {ceiling(resourceLimits.artifact_file_bytes)}
            </li>
            <li>
              軟性時間上限：{limit(resourceLimits.wall_clock_soft_seconds, seconds)}
              （先要求收尾; 硬性上限 {limit(resourceLimits.wall_clock_hard_seconds, seconds)}
              才是強制中止）
            </li>
            <li>Provider 是否 rootless：{provider.rootless ? "是" : "否"}</li>
            <li>Runtime：{provider.runtime ?? "未測量"}</li>
            <li>Runtime 版本：{provider.runtime_version ?? "未測量"}</li>
          </ul>
        </details>
      </dd>
    </>
  );
}
