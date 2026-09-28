export function IncludeTestCasesSection({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <>
      <h2>要不要一起帶走 Test Case</h2>
      <p>
        <label>
          <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />{" "}
          包含可散布的 Test Case 與範例資料
        </label>
      </p>
      <p className="note">
        只有平台策展產生的範例資料會進包。
        <strong>你自己上傳的 Dataset 一律不會進包，也刻意不提供這個選項</strong>
        ——那些檔案的授權判斷不該丟給拿不到判斷材料的人。評估報告、改善建議、Trace 與 Run
        產出同樣不進包：它們是 Run 資料，不是 Skill 內容。
      </p>
    </>
  );
}
