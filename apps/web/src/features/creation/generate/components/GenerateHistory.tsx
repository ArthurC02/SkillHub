import { useGenerateFailures } from "../../generate.service";
import { failureSentence } from "../../generate.model";
import { Timestamp } from "../../../../shared/ui/Timestamp";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";

const GENERATE_FAILURE_LIMIT = 20; // one-number: generateFailureLimit

export function GenerateHistory() {
  const history = useGenerateFailures();

  if (history.isError) {
    return (
      <ReadFailure error={history.error} what="生成失敗紀錄">
        <p className="note" role="status">
          過去的生成紀錄讀取失敗。這不影響你現在能不能生成。
        </p>
      </ReadFailure>
    );
  }
  const failures = history.data?.failures ?? [];
  if (failures.length === 0) return null;

  return (
    <details>
      <summary>最近沒有成功的生成（{failures.length} 次）</summary>
      <ul>
        {failures.map((f) => (
          <li key={f.occurred_at}>
            <Timestamp at={f.occurred_at} />
            {" — "}
            {failureSentence(f)}
          </li>
        ))}
      </ul>
      <p className="note">
        這些是沒有建立任何版本的那幾次，最多列最近 {GENERATE_FAILURE_LIMIT} 次。
        <strong>這裡沒有記下你當時輸入的任務描述</strong>
        ——那份文字跟著它產生的 Skill 走，刪掉 Skill 就跟著刪掉；這份紀錄保存得更久，
        兩邊各留一份等於一個沒有人做過的保存承諾。
      </p>
    </details>
  );
}
