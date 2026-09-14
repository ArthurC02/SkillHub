import { Link } from "@tanstack/react-router";
import { MAX_COMPARE } from "../../../skill";

export function CompareBar({ selected }: { selected: string[] }) {
  return (
    <div className="compare-bar">
      {selected.length >= 2 ? (
        <Link to="/compare" search={{ ids: selected.join(",") }}>
          並排比較這 {selected.length} 個 Skill
        </Link>
      ) : (
        <p className="note">勾選 2 至 {MAX_COMPARE} 個 Skill，即可並排比較它們的靜態資料。</p>
      )}
      {selected.length >= MAX_COMPARE && (
        <p className="note" id="compare-limit">
          已經選滿 {MAX_COMPARE}{" "}
          個，並排比較最多就是這麼多；其餘的勾選框會停用，取消一個才能改選別的。
        </p>
      )}
    </div>
  );
}
