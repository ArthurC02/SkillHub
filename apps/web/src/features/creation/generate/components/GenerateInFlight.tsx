export function GenerateInFlight() {
  return (
    <div role="status" className="notice">
      <p>正在請模型寫這個 Skill，然後用與匯入完全相同的那道驗證檢查它。</p>
      <p>這一步會自己結束，通常十幾秒到一分鐘。</p>
      <p className="note">
        這一段沒有進度可以報——生成是一次呼叫，它要嘛回一個套件要嘛失敗， 沒有中間的量可以顯示。
      </p>
      <p>
        <strong>請不要關掉這個分頁</strong>
        ——這一次生成沒有背景工作可以接手，關掉就等於取消，而且不會留下任何半成品版本。
      </p>
    </div>
  );
}
