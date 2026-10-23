import { useUninvited } from "../../core/session/me.service";

export function BetaInviteNotice() {
  if (!useUninvited()) return null;
  return (
    <p className="notice">
      這個帳號還沒有封測邀請：瀏覽與小工具詳情都開放，複製一份、試跑與下載只開放給受邀的測試者。想試的話，用頁尾的「回報問題」告訴我們。
    </p>
  );
}
