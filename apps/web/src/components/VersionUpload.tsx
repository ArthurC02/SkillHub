import { useState } from "react";
import { ReadFailure } from "./LoginRequired";
import { useSkillVersions } from "../api/skills";
import { useSaveSkillVersion } from "../api/versions";

/**
 * 02:WS-002 的「使用者可以把改過的套件存成下一版」，也就是 `POST /skills/{id}/versions`
 * 的第一個畫面（04 乙-7 的「尺-1」形狀：端點有、測試有、`使用者可以…` 的主詞沒有畫面）。
 *
 * **只給擁有者看**，訊號用這一頁其他三個元件已經在用的那一個：`GET /skills/{id}/versions`
 * 是 workspace-scoped，別人的 Skill 回空清單（ADR-011）。`skill.version` 不是訊號——
 * 它來自那個 Skill 自己的工作區，對每一個訪客都有值（見 `SkillDetail.tsx` 的
 * `PackagingEntry` 檔頭）。React Query 同 key 去重，所以這不是第四個請求。
 *
 * **不是 `.action`**：設計 §4.6.3 的表在 `/skills/$id` 這一列指名的主要動作是
 * 「打包並下載這個版本」，一頁一個填色動作。這顆是次要動作，樣式與其他一般按鈕相同。
 *
 * 失敗走 `ReadFailure` 包住自己那一句（與同頁 `ForkAction` 同一個形狀）：這是 mutation
 * 不是 read，`ReadFailure` 預設的「無法讀取」不對，但伺服器對 400／413／422 各寫了一句
 * 可用的話，而 `apiFetch` 已經把它放進 `ApiError.message`——一句「上傳失敗，請稍後再試」
 * 會把那些全丟掉，也會對 401 說錯話。
 */
export function VersionUpload({ skillId }: { skillId: string }) {
  const versions = useSkillVersions(skillId);
  const save = useSaveSkillVersion(skillId);
  const [file, setFile] = useState<File>();

  // 不在你的工作區（或清單還沒答／答不出來）：不畫表單。這裡沒有 §2.4 的「說出理由」
  // 義務——理由已經由旁邊的 Fork 與試跑兩段講過同一句（「這個 Skill 不在你的工作區，
  // 要先 Fork 一份」），而 §3 第 14 條要的是同一個事實在一頁上只講一次。
  if ((versions.data?.versions.length ?? 0) === 0) return null;

  return (
    <section>
      <h3>上傳新版本</h3>
      <p className="note" data-role="teaching">
        把你改過的套件上傳成這個 Skill 的新版本；舊版本原封不動留著（ADR-003）。
      </p>
      <form
        className="version-upload"
        onSubmit={(event) => {
          event.preventDefault();
          if (file) save.mutate(file);
        }}
      >
        <label htmlFor="skill-version-file">Skill zip</label>
        <input
          id="skill-version-file"
          type="file"
          required
          accept=".zip,application/zip"
          onChange={(event) => setFile(event.target.files?.[0])}
        />
        <button type="submit" disabled={save.isPending}>
          {save.isPending ? "上傳中…" : "上傳成新版本"}
        </button>
      </form>
      {save.isError && (
        <ReadFailure error={save.error} what="上傳新版本">
          <p role="alert">
            上傳沒有成功：{save.error instanceof Error ? save.error.message : String(save.error)}
          </p>
        </ReadFailure>
      )}
      {save.isSuccess && (
        // `duplicate` 是契約自己的欄位（「Identical content returns the existing
        // version with duplicate=true」）。兩件事分開講：多了一版，與什麼都沒多——
        // 後者說成「已建立 v3」是假話。
        <p role="status">
          {save.data.duplicate
            ? `這份內容與現有的 v${save.data.version_number} 完全相同，沒有建立新版本。`
            : `已存成 v${save.data.version_number}。`}
        </p>
      )}
    </section>
  );
}
