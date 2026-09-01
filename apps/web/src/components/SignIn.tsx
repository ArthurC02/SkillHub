import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { API_BASE_URL } from "../api/client";
import { devLogin, useDevLogin } from "../api/me";

/**
 * 登入這個動作，**全 app 只有這一份**。
 *
 * 在此之前它有兩份，`AuthControls` 與 `LoginRequired` 各寫一次，兩份都寫死
 * `<a href=…/auth/github/login>`。在 `02:PORT-005` 講的那台機器上——不能安裝軟體、
 * 對外只保證到得了模型供應商——那個連結**離開產品**，落在瀏覽器的連線錯誤頁。
 * 於是所有需要 session 的東西（匯入、fork、Test Case、試跑、打包、工作區）從畫面
 * 上結構性地到不了，**包括 `04` 丙-114 寫下的示範走法本身**（「以 `seed-importer`
 * 的身分展示」——那個身分只有離線端點生得出來）。
 *
 * `DEV_LOGIN=1` 由 `tools/cleanmode/start.mjs` 設，`POST /auth/dev/login` 一直都
 * 掛著、一直都會動。**缺的從來不是能力，是畫面上沒有人去呼叫它。**
 *
 * 合成一份的理由與 `PORT-003` 的揭露同一條（「不得逐頁各寫一份」）：三份文案裡總
 * 有一份日後會變成安慰話，而這一份如果漏改，漏掉的那一頁在離線機器上就又沒有出路。
 */
export function SignInAction() {
  const offline = useDevLogin();
  if (!offline) {
    return <a href={`${API_BASE_URL}/auth/github/login`}>使用 GitHub 登入</a>;
  }
  return <OfflineSignIn />;
}

/**
 * 離線登入。名字任意——這是 ADR-020 的離線 provider，不是身分驗證，
 * `start.mjs` 的警告逐字說過「anybody can sign in as any name without a credential」。
 *
 * 預設填 `seed-importer`，因為在這個模式下它不是一個隨便的名字：**目錄工作區就是
 * 它的工作區**，所以只有它能直接對策展 Skill 建 Test Case 並且真的跑起來
 * （`04` 丙-114 實測 `queued → running → succeeded`）。換成別的名字一樣登得進去，
 * 但派送閘門會擋下非策展內容——那是 `02:PORT-010` 的邊界，不是登入的問題。
 */
function OfflineSignIn() {
  const [user, setUser] = useState("seed-importer");
  const queryClient = useQueryClient();
  const signIn = useMutation({
    mutationFn: () => devLogin(user.trim()),
    // 與 AuthControls 的登出對稱：session 換了人，之前那個人的每一筆快取都不再
    // 是這個工作區的答案。
    onSuccess: () => queryClient.clear(),
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        signIn.mutate();
      }}
    >
      <label htmlFor="offline-user">離線登入（這台機器沒有 GitHub 可以連）</label>{" "}
      <input
        id="offline-user"
        value={user}
        onChange={(e) => setUser(e.target.value)}
        autoComplete="off"
      />{" "}
      <button type="submit" disabled={signIn.isPending}>
        登入
      </button>
      {/*
        設計 §2.2 第三向：擋住人的訊息要說下一步。這裡的下一步不是「再試一次」，
        而是「這個部署沒有掛離線登入」——但那不可能發生在這個分支，因為畫出這個
        表單的前提就是旗標為真。所以失敗只會是伺服器端的意外，照實說。
      */}
      {signIn.error && <span role="alert">登入失敗：{signIn.error.message}</span>}
    </form>
  );
}
