export function AllowedToolsSection({
  allowedTools,
  scanStatus,
}: {
  allowedTools: string[] | undefined;
  scanStatus: "scanned" | "unavailable";
}) {
  if (allowedTools && allowedTools.length > 0) {
    return (
      <>
        <ul>
          {allowedTools.map((tool) => (
            <li key={tool}>
              <code>{tool}</code>
            </li>
          ))}
        </ul>
        <p className="note">以上為套件自行宣告的 allowed-tools，未經驗證。</p>
      </>
    );
  }
  if (scanStatus === "unavailable") {
    return (
      <p className="note">
        未測量——這個版本沒有靜態掃描結果可讀，所以平台不知道套件宣告了哪些工具。
      </p>
    );
  }
  return (
    <p className="note">
      不適用——套件沒有宣告 allowed-tools。在 Agent 小工具的格式裡那代表
      <strong>不設限</strong>，不代表它不用工具。
    </p>
  );
}
