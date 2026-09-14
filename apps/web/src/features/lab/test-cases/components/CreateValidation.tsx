export function CreateValidation({
  skillId,
  name,
  prompt,
}: {
  skillId: string;
  name: string;
  prompt: string;
}) {
  const missing = [
    skillId === "" ? "選一個 Skill" : "",
    name === "" ? "填名稱" : "",
    prompt.trim() === "" ? "寫 User Prompt" : "",
  ].filter((s) => s !== "");

  if (missing.length === 0) return null;
  return (
    <p className="note" role="status" id="create-why">
      還不能建立，因為：{missing.join("、")}。三個都是必填。
    </p>
  );
}
