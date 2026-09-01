import type { ReactNode } from "react";
import { ApiError } from "../api/client";
import { SignInAction } from "./SignIn";

/**
 * 資訊架構 §5 IA-6（2026-08-25 裁定）：**登出狀態不由 router 守衛，由 401 這個
 * 具名狀態自己說。**
 *
 * `RequireSession`（`creator/workspace/http.go`）對每一個需要 session 的讀取回
 * `401 {"error":"not authenticated"}`，而十二處呼叫點把 `error.message` 直接內插
 * 進中文句子裡，於是畫面上是「無法讀取你的 Skill 清單：not authenticated」。
 *
 * 這裡不做轉址、不做全域 toast、不動導覽列——三者的理由逐條寫在該裁定裡，最短的
 * 一條是：轉址會弄丟訪客被寄來的那個位址。
 *
 * 形狀是**一個判斷加兩個元件**，因為呼叫點有兩種形狀而不是一種：
 *
 *  - **讀取失敗之後**（`ReadFailure`）：頁面已經在畫「無法讀取 X：{message}」，
 *    換成一行呼叫，401 說登入、其餘一字不改。**非 401 不得被吞掉**——500 仍然
 *    要說出是什麼壞了。
 *  - **請求發出之前**（`LoginRequired`）：頁面把一整份可操作的表單畫給訪客，等他
 *    做完事才拒絕。設計系統 §2.2 與 §2.4 要求會被拒絕的控制項在**被使用之前**
 *    就說，所以那些頁面自己讀 `useMe()` 並在表單的位置畫這一句。
 *
 * `role="status"` 而不是 `role="alert"`：401 是可預期的狀態，不是錯誤。前例是
 * `SkillFiles.tsx` 對 403 的處理。句子帶著登入動作本身（設計 §2.2 第三向：擋住人
 * 的訊息必須說下一步是什麼），連結與 `AuthControls` 用的是同一個入口。
 *
 * 不加 class：它取代的那一行本來就沒有 class，而 §2.7 的守衛要求新 class 得有
 * 真的規則。沒有視覺需求就不發明一個。
 */

/**
 * 這個讀取是因為「沒有 session」被拒的。
 *
 * 原本是 `AuthControls.tsx:15` 的區域判斷，也是全 app 唯一一處 `status === 401`
 * 的判斷。提出來，是因為它本來就是每一頁都需要的那一個問題。
 */
export function unauthenticated(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401;
}

/** 「這個東西要登入才拿得到」，外加登入動作本身。 */
export function LoginRequired({ what }: { what: string }) {
  return (
    <p role="status">
      {what}需要登入。
      <SignInAction />
    </p>
  );
}

export function ReadFailure({
  error,
  what,
  children,
}: {
  error: unknown;
  /** 主詞。401 時是「{what}需要登入」，其餘是「無法讀取{what}：…」。 */
  what: string;
  /**
   * 這一頁對**非 401** 的既有說法，當它不是「無法讀取{what}：{message}」時。
   * 由呼叫端在 `error` 為真的前提下渲染，所以裡面讀 `error.message` 是安全的。
   */
  children?: ReactNode;
}) {
  if (!error) return null;
  if (unauthenticated(error)) return <LoginRequired what={what} />;
  if (children) return <>{children}</>;
  return (
    <p role="alert">
      無法讀取{what}：{error instanceof Error ? error.message : String(error)}
    </p>
  );
}
