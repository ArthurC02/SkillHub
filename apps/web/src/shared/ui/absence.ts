export const ABSENCE_WORDS = ["不適用", "未測量", "測量失敗", "無權檢視", "尚未定值"] as const;

export type Absence = (typeof ABSENCE_WORDS)[number];

export const IN_PROGRESS = "處理中";
