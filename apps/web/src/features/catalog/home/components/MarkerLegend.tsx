export function MarkerWarning() {
  return (
    <p className="note">
      「AI 改寫」與「AI 產生」由模型寫成，未經人工核對；你的 Agent 讀的是套件自己的 description，
      不是這裡的改寫。
    </p>
  );
}

export function MarkerLegend() {
  return (
    <p className="note">
      標記說明：「作者原文」是套件的 frontmatter description；「規則產生」依查詢與文件的關鍵字
      重疊組出；「來源未標示」代表伺服器沒有回報這段摘要的來源。
    </p>
  );
}
