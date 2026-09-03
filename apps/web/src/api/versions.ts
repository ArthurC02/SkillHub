import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ImportResult } from "./import";

/**
 * `POST /skills/{id}/versions`（`saveSkillVersion`，02:WS-002）——契約裡有、路由裡
 * 有、`apps/web/src` 裡一個呼叫點都沒有的那個端點。
 *
 * 它補的是「Fork → 改 → 存回同一個 Skill」這條路的最後一段：在此之前，改一份自己
 * 的套件唯一走得完的做法是「打包下載 → 本機改 → 再匯入成**另一個** Skill」，而那
 * 會把 lineage 斷在中間（新的 skills 列，沒有 `forked_from_skill_id`）。
 *
 * **手寫 `apiFetch` 而不是 `packages/api-client-ts` 的產生器 client**，理由與
 * `api/skills.ts` 檔頭那段逐字相同：產生的 client 是 camelCase 加一層 runtime
 * 轉換，改用它是一次遷移而不是順手整理；`contract.test.ts` 才是讓 `api/types.ts`
 * 與契約保持同步的那個機器。這裡的 body 形狀直接照契約：`application/zip`，
 * 整個檔案就是 body，沒有 multipart、沒有欄位。
 *
 * 回應是 `UploadResult`，與 `/skills/import/*` 同一個 schema，所以型別沿用
 * `api/import.ts` 的 `ImportResult` 而不是再宣告一份會漂移的複本。
 */
export function saveSkillVersion(skillId: string, file: File) {
  return apiFetch<ImportResult>(`/skills/${skillId}/versions`, {
    method: "POST",
    headers: { "Content-Type": "application/zip" },
    body: file,
  });
}

/**
 * 成功後失效的是 `["skills", skillId]` 這個**前綴**：它同時涵蓋詳情
 * （`["skills", skillId]`）與版本清單（`["skills", skillId, "versions"]`），而這一次
 * 寫入把兩者都改舊了——`skill.version` 指的是最新一版，版本清單多一列。
 *
 * 前綴刻意帶著 `skillId`，不是裸的 `["skills"]`：後者會一併命中
 * `["skills","search",…]`，而重跑搜尋會讓伺服器再寫一筆 `search_performed`——
 * 也就是 ⛔ 邊界在保護的那個漏斗數字（`api/skills.ts` 的 `useForkSkill` 記著同一個
 * 陷阱）。
 */
export function useSaveSkillVersion(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => saveSkillVersion(skillId, file),
    onSuccess: () => client.invalidateQueries({ queryKey: ["skills", skillId] }),
  });
}
