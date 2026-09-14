import { useState } from "react";
import { useAccountLookup } from "../admin.service";
import { ApiError } from "../../../core/api/client";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import { AdminPage } from "../components/AdminPage";
import { AccountCard } from "./components/AccountCard";

const notFound = (error: unknown) => error instanceof ApiError && error.status === 404;

export function AdminAccounts() {
  const [draft, setDraft] = useState("");
  const [email, setEmail] = useState("");
  const account = useAccountLookup(email);

  return (
    <AdminPage
      heading="帳號與點數"
      lede="每查到一次帳號、每讀一次點數，都會留下一筆紀錄：誰在何時查了誰。"
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          setEmail(draft.trim());
        }}
      >
        <div className="field">
          <label htmlFor="admin-account-email">Email</label>
          <input
            id="admin-account-email"
            type="email"
            required
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
        </div>
        <button type="submit" className="action">
          查詢
        </button>
      </form>
      {account.isFetching && <Loading what="帳號" />}
      {notFound(account.error) ? (
        <p role="status">沒有 email 是「{email}」的帳號。已刪除的帳號查不到。</p>
      ) : (
        <ReadFailure error={account.error} what="帳號" />
      )}
      {account.data && !account.isFetching && <AccountCard account={account.data} />}
    </AdminPage>
  );
}
