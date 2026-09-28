import { EVALUATION_POLL_MAX_404 } from "../../evaluation.service";

export function NotEvaluatedNotice({
  showPollingNote,
  stoppedAsking,
}: {
  showPollingNote: boolean;
  stoppedAsking: boolean;
}) {
  return (
    <div className="notice">
      <p>
        <strong>未評估</strong>
      </p>
      <p>這個 Run 沒有評估結果。未評估不等於通過，也不等於未通過。</p>
      {showPollingNote && !stoppedAsking && (
        <p className="note">
          這一頁每 3 秒再查一次；如果有評估正在排隊，結果會自己出現在這裡，不必重新整理。
        </p>
      )}
      {showPollingNote && stoppedAsking && (
        <p className="note">
          這一頁已經停止再查了——查了 {EVALUATION_POLL_MAX_404}{" "}
          次都還是沒有評估，所以它不會再自己更新。 這通常表示沒有人替這個 Run
          送出評估，而不是評估失敗。重新整理這一頁會再查一次。
        </p>
      )}
    </div>
  );
}
