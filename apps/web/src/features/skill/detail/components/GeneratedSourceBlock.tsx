import { Timestamp } from "../../../../shared/ui/Timestamp";
import { Link } from "@tanstack/react-router";
import type { SkillSource } from "../../../../core/api/types";

export function GeneratedSourceBlock({ source }: { source: SkillSource }) {
  const diagram = source.generation_inputs?.diagram;
  const references = source.generation_inputs?.references;
  return (
    <>
      <p>
        {source.task_description
          ? "來源：由平台依你的任務描述生成"
          : diagram
            ? "來源：由平台依你上傳的流程圖生成"
            : "來源：由平台生成"}
      </p>
      {source.task_description && (
        <details>
          <summary>你當時輸入的任務描述</summary>
          <p>{source.task_description}</p>
        </details>
      )}
      {source.generation_inputs && (
        <details>
          <summary>這一次生成用到的輸入</summary>
          {diagram && (
            <p>
              流程圖：{diagram.media_type}，{diagram.bytes} bytes，sha256{" "}
              <code>{diagram.sha256}</code>
              <span className="note">（平台沒有保留圖片本身，只留下這個雜湊）</span>
            </p>
          )}
          {references && references.length > 0 && (
            <>
              <p>參考的 Skill：</p>
              <ul>
                {references.map((r) => (
                  <li key={r.version_id}>
                    <Link to="/skills/$skillId" params={{ skillId: r.skill_id }}>
                      {r.name}
                    </Link>
                  </li>
                ))}
              </ul>
            </>
          )}
        </details>
      )}
      {source.fetched_at && (
        <p>
          生成時間：
          <Timestamp at={source.fetched_at} />
        </p>
      )}
      {source.generator_model && (
        <p>
          模型：<code>{source.generator_model}</code>
        </p>
      )}
      {source.generator_prompt_version && (
        <p>
          提示詞版本：<code>{source.generator_prompt_version}</code>
        </p>
      )}
      {source.content_hash && (
        <details>
          <summary>內容雜湊</summary>
          <code>{source.content_hash}</code>
        </details>
      )}
    </>
  );
}
