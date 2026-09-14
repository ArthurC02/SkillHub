import { useState } from "react";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import { useSetSkillCategory, useSkillVersions } from "../../skills.service";
import type { SetSkillCategoryRequest } from "@skillhub/api-client-ts";
import type { Labelled } from "../../../../core/api/types";

const CATEGORY_CHOICES: { value: SetSkillCategoryRequest["category"]; label: string }[] = [
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
  { value: "unassigned", label: "尚未定值" },
];

export function CategoryEditor({ skillId, category }: { skillId: string; category: Labelled }) {
  const versions = useSkillVersions(skillId);
  const [choice, setChoice] = useState<SetSkillCategoryRequest["category"]>(
    category.value as SetSkillCategoryRequest["category"],
  );
  const save = useSetSkillCategory(skillId);

  if ((versions.data?.versions.length ?? 0) === 0) return null;

  return (
    <section>
      <h3>類別</h3>
      <p className="field">
        <label htmlFor="skill-category">這個 Skill 是做什麼用的</label>
        <select
          id="skill-category"
          value={choice}
          onChange={(e) => setChoice(e.target.value as SetSkillCategoryRequest["category"])}
        >
          {CATEGORY_CHOICES.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>
      </p>
      <button type="button" onClick={() => save.mutate(choice)} disabled={save.isPending}>
        {save.isPending ? "儲存中…" : "儲存"}
      </button>
      {save.isError && (
        <ReadFailure error={save.error} what="設定類別">
          <p role="alert">類別沒有設定成功，可以再按一次。</p>
        </ReadFailure>
      )}
      {save.isSuccess && <p role="status">類別已更新。</p>}
    </section>
  );
}
