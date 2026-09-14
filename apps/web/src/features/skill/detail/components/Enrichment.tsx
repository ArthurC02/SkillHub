import type { SkillEnrichment, SkillTags } from "../../../../core/api/types";

const TAG_BUCKETS: Array<{ key: keyof SkillTags; label: string }> = [
  { key: "inputs", label: "輸入" },
  { key: "outputs", label: "輸出" },
  { key: "tools", label: "會用到的工具" },
  { key: "dependencies", label: "依賴" },
];

export function Enrichment({ enrichment }: { enrichment: SkillEnrichment }) {
  if (enrichment.status !== "enriched") {
    return (
      <>
        <p className="note">{enrichment.note}</p>
        {TAG_BUCKETS.map(({ key, label }) => (
          <p key={key}>
            {label}：<span className="note">未知（尚未產生索引摘要）</span>
          </p>
        ))}
      </>
    );
  }

  return (
    <>
      <p className="badge-row">
        <span className="badge badge-source-model">AI 產生</span>
      </p>
      {enrichment.summary && <p>{enrichment.summary}</p>}

      {enrichment.tags &&
        TAG_BUCKETS.map(({ key, label }) =>
          enrichment.tags![key].length > 0 ? (
            <p key={key}>
              {label}：
              <span className="tag-list">
                {enrichment.tags![key].map((tag) => (
                  <span key={tag} className="badge">
                    {tag}
                  </span>
                ))}
              </span>
            </p>
          ) : (
            <p key={key}>
              {label}：<span className="note">未測量（沒有擷取到，不代表沒有）</span>
            </p>
          ),
        )}

      {enrichment.task_examples && enrichment.task_examples.length > 0 && (
        <details>
          <summary>可以用來做什麼（AI 產生的任務範例）</summary>
          <ul>
            {enrichment.task_examples.map((example) => (
              <li key={example}>{example}</li>
            ))}
          </ul>
        </details>
      )}

      <p className="note">{enrichment.note}</p>
      {(enrichment.model || enrichment.prompt_version) && (
        <details>
          <summary>產生這段摘要的模型</summary>
          <ul>
            {enrichment.model && (
              <li>
                模型：<code>{enrichment.model}</code>
              </li>
            )}
            {enrichment.prompt_version && (
              <li>
                Prompt 版本：<code>{enrichment.prompt_version}</code>
              </li>
            )}
          </ul>
        </details>
      )}
    </>
  );
}
