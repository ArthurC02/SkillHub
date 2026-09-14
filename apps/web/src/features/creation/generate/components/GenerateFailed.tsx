import type { GenerateRejected } from "../../../../core/api/types";
import { Findings } from "../../../../shared/ui/Findings";

export function GenerateFailed({
  rejected,
  onRetry,
}: {
  rejected: GenerateRejected;
  onRetry: () => void;
}) {
  return (
    <section role="alert">
      <h3>生成失敗：套件被擋下，沒有建立任何版本</h3>
      <p className="note">
        {rejected.attempts > 1
          ? "平台已經自動用同一段描述再試過一次，第二次仍然沒有通過。下面是檢查逐字回報的內容，沒有經過改寫。"
          : "這一次沒有自動重試——被擋下的原因不是排版手滑，同一段描述再送一次會得到同樣的結果。下面是檢查逐字回報的內容，沒有經過改寫。"}
      </p>
      <Findings findings={rejected} level={4} />
      <p>
        <button type="button" onClick={onRetry}>
          再試一次
        </button>{" "}
        或者改寫上面的任務描述再送出——把要做什麼、輸入是什麼、預期產出是什麼寫得更具體，通常比重試有用。
      </p>
    </section>
  );
}
