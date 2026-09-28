import { EVALUATION_POLL_MAX_PENDING } from "../../evaluation.service";

export function EvaluatingNotice({ pendingPollStopped }: { pendingPollStopped: boolean }) {
  return (
    <div className="notice" role="status">
      <p>
        <strong>評估進行中</strong>
        {pendingPollStopped
          ? "——這一筆評估說自己還在做，但這一頁已經停止再查了。"
          : "——判定還在做。它會自己完成，不需要你回來按任何東西。"}
      </p>
      <p className="note">可以關掉這一頁（平台在跑，不是你的瀏覽器）</p>
      <p className="note">
        這一段沒有進度可以報——評審不是分批完成的，它要嘛給出判定要嘛失敗，
        而兩種結果都會出現在這裡。
        {pendingPollStopped ? (
          <>
            這一頁查了 {EVALUATION_POLL_MAX_PENDING} 次、約 {(EVALUATION_POLL_MAX_PENDING * 3) / 60}{" "}
            分鐘，狀態都還是「進行中」，所以它不會再自己更新了。 停下來的是這一頁的查詢，不是那個
            job：重新整理這一頁會再查一次。 一直停在這裡代表那個工作沒有在推進，而不是判定為未通過。
          </>
        ) : (
          "這一頁每 3 秒自己查一次。"
        )}
      </p>
    </div>
  );
}
