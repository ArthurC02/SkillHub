import "./RouteFailure.css";

export function RouteFailure() {
  return (
    <section
      className="notice notice-danger route-failure"
      role="alert"
      aria-labelledby="route-failure-title"
    >
      <h1 id="route-failure-title">這一頁暫時無法顯示</h1>
      <p>畫面發生未預期的問題。請重新整理再試，或回到目錄繼續瀏覽。</p>
      <p className="route-failure-actions">
        <a href={window.location.href}>重新整理</a>
        <a href="/">回目錄</a>
      </p>
    </section>
  );
}
