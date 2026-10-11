import { Link } from "@tanstack/react-router";
import { GraderManager } from "../components/GraderManager";

export function EvaluationGraders() {
  return (
    <section>
      <h1>評分器</h1>
      <p className="note">
        評分器是一段寫給評審模型的評分指示。驗收條件的檢查可以選用平台通用的評分器，或這個 Workspace
        自己的評分器；這裡只能新增與修改這個 Workspace 的。
      </p>
      <p>
        <Link to="/lab/test-cases" search={{}}>
          回到試跑情境
        </Link>
      </p>
      <GraderManager scope="workspace" />
    </section>
  );
}
