export function RouteError() {
  return (
    <>
      <h1>這一頁暫時無法顯示</h1>
      <p role="alert">載入或顯示頁面時發生問題。你可以重新載入這一頁，或從導覽列前往其他功能。</p>
      <p id="route-reload-consequence" className="note">
        重新載入會清除尚未送出的內容。
      </p>
      <button aria-describedby="route-reload-consequence" onClick={() => window.location.reload()}>
        重新載入頁面
      </button>
    </>
  );
}
