import type { PackagingPreview } from "../../packaging.service";

export function Dependencies({ preview }: { preview: PackagingPreview }) {
  // server may send null for an empty list (Go nil slice), not []
  const dependencies = preview.dependencies ?? [];
  return (
    <>
      <h3>依賴需求</h3>
      {dependencies.length === 0 ? (
        <p className="note">
          {preview.allowed
            ? "這個套件沒有宣告依賴檔，程式碼裡也沒有掃到第三方 import。這是靜態掃描的結果，不是作者的保證——掃描不執行套件裡的任何東西。"
            : "還沒有讀到套件內容（上面那道鎖先擋下了），所以這裡不是「沒有依賴」，是還沒有東西可以看。"}
        </p>
      ) : (
        <>
          <ul className="risk-list">
            {dependencies.map((d) => (
              <li key={d}>{d}</li>
            ))}
          </ul>
          <p className="note">
            Skill Hub 不會替你安裝這些，
            打包與掃描階段也不執行套件內的任何程式碼——你的環境有沒有這些依賴，要你自己確認。
          </p>
        </>
      )}
    </>
  );
}
