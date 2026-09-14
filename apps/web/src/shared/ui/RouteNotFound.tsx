import { Link } from "@tanstack/react-router";

export function RouteNotFound() {
  return (
    <>
      <h1>這一頁現在不存在</h1>
      <p>
        網址可能打錯了，或這一頁已經不在了。你可以回到{" "}
        <Link to="/" search={{}}>
          目錄
        </Link>
        。
      </p>
    </>
  );
}
