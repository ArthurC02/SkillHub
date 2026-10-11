import { GraderManager } from "../../evaluation";
import { AdminPage } from "../components/AdminPage";

export function AdminGraders() {
  return (
    <AdminPage
      heading="平台評分器"
      lede="每個 Workspace 都能在驗收條件的檢查裡選用這裡的評分器；每次修改存成新版本並寫進動作紀錄。"
    >
      <GraderManager scope="platform" />
    </AdminPage>
  );
}
