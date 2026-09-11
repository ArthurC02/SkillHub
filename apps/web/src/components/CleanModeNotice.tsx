import { useCleanMode } from "../api/me";

export function CleanModeNotice({ admin = false }: { admin?: boolean }) {
  const cleanMode = useCleanMode();
  if (!cleanMode) return null;

  return (
    <>
      {admin && (
        <p className="notice">
          淨測試模式下任何人都能以 operator 登入：這個後台的每一顆按鈕，此刻誰都按得到。
        </p>
      )}
      <details className="clean-mode-notice">
        <summary className="notice">
          淨測試模式：5 項在這個模式下不成立（沒有隔離、不驗簽章、只有一條連線）
        </summary>
        <p className="note">
          沙箱沒有隔離——不是比較弱的隔離，是沒有邊界。這個模式只跑策展過的展示素材。
        </p>
        <p className="note">
          物件儲存不驗證 presigned URL。按下去有檔案出來，不構成任何授權上的證明。
        </p>
        <p className="note">
          資料庫只有一條連線，併發語意與生產不同。「在這個模式下沒重現」不是一個結論。
        </p>
        <p className="note">物件儲存只在記憶體裡，行程結束即消失。</p>
        <p className="note">
          試跑前那份「可連往哪裡」的清單，在這個模式下不被強制——沒有任何東西擋著沙箱連別的地方。
        </p>
      </details>
    </>
  );
}
