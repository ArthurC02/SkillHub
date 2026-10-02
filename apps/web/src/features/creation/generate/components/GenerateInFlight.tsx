import { useEffect, useState } from "react";

export function GenerateInFlight() {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const started = Date.now();
    const timer = window.setInterval(
      () => setElapsed(Math.floor((Date.now() - started) / 1000)),
      1000,
    );
    return () => window.clearInterval(timer);
  }, []);
  return (
    <div role="status" className="notice">
      <p>正在產生並驗證小工具；完成後會在這裡顯示結果。</p>
      <p>這次沒有可恢復的背景工作，請保持分頁開啟。</p>
      {elapsed >= 10 && (
        <p aria-live="off">已等待 {elapsed} 秒；仍在等候結果，無法提供完成百分比。</p>
      )}
    </div>
  );
}
