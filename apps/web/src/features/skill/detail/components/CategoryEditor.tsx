import { useState } from "react";
import { ApiError, isWriteResultUnknown } from "../../../../core/api/client";
import { ReadFailure } from "../../../../shared/ui/LoginRequired";
import {
  useSetSkillCategory,
  useSkillDetail,
  useSkillVersions,
  type SkillCategoryChoice,
} from "../../skills.service";

const CATEGORY_CHOICES: { value: SkillCategoryChoice; label: string }[] = [
  { value: "documents", label: "文件" },
  { value: "writing", label: "寫作" },
  { value: "data", label: "資料" },
  { value: "unassigned", label: "尚未定值" },
];

type CategoryContext = {
  skillId: string;
  skill: ReturnType<typeof useSkillDetail>;
};

export function CategoryEditor({ skillId, skill }: CategoryContext) {
  const versions = useSkillVersions(skillId);
  const denied = [skill.error, versions.error].some(
    (error) => error instanceof ApiError && [401, 403, 404, 410].includes(error.status),
  );
  if (!skill.data || !versions.data?.versions.length || denied) return null;
  return <CategoryForm key={skillId} skillId={skillId} skill={skill} versions={versions} />;
}

function CategoryForm({
  skillId,
  skill,
  versions,
}: CategoryContext & {
  versions: ReturnType<typeof useSkillVersions>;
}) {
  const [draft, setDraft] = useState<SkillCategoryChoice>();
  const choice =
    draft ?? CATEGORY_CHOICES.find((item) => item.value === skill.data?.category.value)?.value;
  const save = useSetSkillCategory(skillId);
  const refreshing = skill.fetchStatus !== "idle" || versions.fetchStatus !== "idle";
  const confirmed = skill.isSuccess && versions.isSuccess && !refreshing;
  const paused = skill.fetchStatus === "paused" || versions.fetchStatus === "paused";

  return (
    <section aria-labelledby="skill-category-heading">
      <h2 id="skill-category-heading">類別</h2>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (confirmed && choice && !save.isPending) save.mutate(choice);
        }}
      >
        <p className="field">
          <label htmlFor="skill-category">這個小工具是做什麼用的</label>
          <select
            id="skill-category"
            value={choice ?? ""}
            required
            onChange={(event) => {
              const selected = CATEGORY_CHOICES.find((item) => item.value === event.target.value);
              if (selected) setDraft(selected.value);
            }}
          >
            {!choice && (
              <option value="" disabled>
                請選擇類別
              </option>
            )}
            {CATEGORY_CHOICES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </p>
        <button type="submit" disabled={!confirmed || !choice || save.isPending}>
          {save.isPending ? "儲存中…" : "儲存"}
        </button>
      </form>
      {(!confirmed || save.isError) && (
        <>
          {!confirmed && (
            <p role="status">
              {paused
                ? "等待連線以確認編輯條件。"
                : "尚未重新確認小工具資訊與擁有的版本，暫時無法儲存類別。"}
            </p>
          )}
          <button
            type="button"
            disabled={refreshing}
            onClick={() => {
              void skill.refetch();
              void versions.refetch();
              document.getElementById("main-content")?.focus();
            }}
          >
            {refreshing ? "重新確認中…" : "重新確認編輯條件"}
          </button>
        </>
      )}
      <CategorySaveResult save={save} />
    </section>
  );
}

function CategorySaveResult({ save }: { save: ReturnType<typeof useSetSkillCategory> }) {
  const submitted = CATEGORY_CHOICES.find((item) => item.value === save.variables)?.label;
  return (
    <>
      {save.isPending && <p role="status">正在儲存「{submitted}」類別。</p>}
      {save.isError && (
        <ReadFailure error={save.error} what="設定類別">
          <p role="alert">
            {isWriteResultUnknown(save.error)
              ? `尚未確認「${submitted}」類別是否已儲存。請先重新確認編輯條件，再決定是否重新儲存目前選擇。`
              : `「${submitted}」類別的儲存請求未獲接受。請確認編輯條件後再試。`}
          </p>
        </ReadFailure>
      )}
      {save.isSuccess && <p role="status">類別已更新為「{submitted}」。</p>}
    </>
  );
}
