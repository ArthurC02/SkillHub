import { AgentAvatar } from "./ConversationLog";

const STARTERS = [
  {
    title: "會議記錄 → 待辦清單",
    desc: "從逐字稿抓出待辦、負責人和期限",
    prompt: "把會議逐字稿整理成待辦清單，每一項要有負責人和期限；沒講到期限就標「未定」。",
  },
  {
    title: "客服來信分類",
    desc: "依問題類型分類，並草擬第一版回覆",
    prompt: "把客服來信依問題類型分類，並為每一封草擬第一版回覆。",
  },
  {
    title: "發票資料擷取",
    desc: "抓出金額、日期與統一編號",
    prompt: "從發票內容擷取金額、開立日期與統一編號，輸出成一張表格。",
  },
  {
    title: "PR → 版本說明",
    desc: "把合併的 PR 整理成給使用者看的更新說明",
    prompt: "把這週合併的 PR 描述整理成給使用者看的版本更新說明，依功能分組。",
  },
];

export function SessionEmptyState({
  busy,
  onPick,
}: {
  busy: boolean;
  onPick: (prompt: string) => void;
}) {
  return (
    <>
      <p className="system-line">Agent 會先和你確認需求與驗收條件，才開始寫草稿</p>
      <ol className="creation-log">
        <li data-role="assistant">
          <AgentAvatar />
          <span className="creation-who">Agent</span>
          <span className="creation-text">
            想做一個什麼樣的小工具？說說它要完成什麼，也可以附上流程圖。
          </span>
        </li>
      </ol>
      <ul className="starter-cards" aria-label="可以這樣開始">
        {STARTERS.map((s) => (
          <li key={s.title}>
            <button type="button" disabled={busy} onClick={() => onPick(s.prompt)}>
              <strong>{s.title}</strong>
              <span>{s.desc}</span>
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}
