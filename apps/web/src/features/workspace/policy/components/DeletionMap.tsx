import { Link } from "@tanstack/react-router";

export function DeletionMap() {
  return (
    <ul className="risk-list">
      <li>
        <Link to="/library">資產庫</Link>
        ：刪掉一個小工具。版本快照會凍結保留，不隨這次刪除消失，誤刪還有救；別人複製一份
        過的版本不受影響。
      </li>
      <li>
        <Link to="/workspace/downloads">下載紀錄</Link>
        ：刪掉打包好的檔案。「你下載過幾次」的紀錄會留著，因為那件事發生過。
      </li>
      <li>
        <Link to="/workspace/runs">試跑紀錄</Link>：進到某一次試跑紀錄
        可以刪掉它的產出檔案。執行紀錄與評估判定保留，引用過該檔案的評估會顯示證據已不存在。
      </li>
      <li>
        <Link to="/workspace/account">帳號</Link>
        ：刪掉整個帳號。申請之後有一段寬限期，期間隨時可以取消，實際結束日期由伺服器算出來顯示在
        那一頁；哪些會實體刪除、哪些會保留但去掉你的身分，也由伺服器在申請後逐條列出。
      </li>
    </ul>
  );
}
