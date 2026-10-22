import { MISSING_EVALUATION_RECOVERY } from "../evaluation.model";

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
      <p>目前尚未取得這次試跑的評估結果。未評估不等於通過，也不等於未通過。</p>
      {showPollingNote && !stoppedAsking && (
        <p className="note">這一頁每 3 秒再查一次；若平台提供評估，結果會自己出現在這裡。</p>
      )}
      {showPollingNote && stoppedAsking && (
        <p className="note" role="status">
          已停止自動確認評估結果。可手動重新確認；這不代表評估失敗。{MISSING_EVALUATION_RECOVERY}
        </p>
      )}
    </div>
  );
}
