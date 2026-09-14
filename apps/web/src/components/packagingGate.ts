import type { PackagingBlockedReason } from "../api/packaging";
import type { Redistribution, SkillDetail } from "../api/types";

export const PACKAGING_BLOCKED_LABEL: Record<PackagingBlockedReason, string> = {
  license_hold:
    "這個 Skill 正在授權審查中（人工暫時保留）。審查期間平台不產出任何套件，標準套件也不例外。",
  not_redistributable:
    "這個 Skill 的授權不允許再散布，平台不會把它交出去。授權已人工確認，不等於可以再散布。沒有讓你自己解除這道鎖的路徑——它擋的是授權本身說的話。",
  license_unknown:
    "沒有人確認過這個 Skill 可不可以再散布。授權未知一律當成不可散布處理——這不是等待中的暫時狀態，是預設就擋。目前沒有讓你自己解除它的路徑：放行需要具名的授權來源證據，只有平台管理者改得動（ADR-057）。",
  validation_blocked:
    "用這些設定打出來的套件，過不了平台自己匯入時要過的驗證，因此不能標示為有效套件。下面的錯誤清單就是要修的東西。",
  file_removed_by_packager:
    "SKILL.md 指向的檔案被打包器排除了，所以這一份下載回去會缺少它自己說明要用的東西——平台不交出一個自己弄殘的套件。下面「平台的說法」會指名是哪個檔；把它移出被排除的目錄、或用實體檔案取代連結，就可以再打包一次。",
};

export const REDISTRIBUTION_GATE: Record<Redistribution, PackagingBlockedReason | null> = {
  allowed: null,
  self_supplied: null,
  generated: null,
  blocked: "not_redistributable",
  unknown: "license_unknown",
};

export function packagingGate(skill: SkillDetail): PackagingBlockedReason | null {
  if (skill.access_restriction) return "license_hold";
  const value = skill.redistribution?.value;
  if (value === undefined || !Object.prototype.hasOwnProperty.call(REDISTRIBUTION_GATE, value)) {
    return "license_unknown";
  }
  return REDISTRIBUTION_GATE[value as Redistribution];
}
