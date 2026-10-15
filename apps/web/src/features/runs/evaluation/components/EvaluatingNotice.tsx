import { Timestamp } from "../../../../shared/ui/Timestamp";

export function EvaluatingNotice({
  pendingPollStopped,
  updatedAt,
}: {
  pendingPollStopped: boolean;
  updatedAt: number;
}) {
  return (
    <div className="notice">
      <p role="status">
        <strong>{pendingPollStopped ? "尚未確認完成判定" : "評估進行中"}</strong>
        {pendingPollStopped
          ? "——已達自動查詢上限，請手動確認目前狀態。"
          : "——平台自動處理判定，完成或失敗後會保存結果。"}
      </p>
      <p className="note">沒有可顯示的中間進度。關閉此頁不會取消平台上的評估。</p>
      <p className="note">
        評估狀態上次取得於 <Timestamp at={new Date(updatedAt).toISOString()} relative />
      </p>
      <p className="note">
        {pendingPollStopped
          ? "已停止自動確認評估結果；這不代表評估失敗。"
          : "這一頁每 3 秒自己查一次。"}
      </p>
    </div>
  );
}
