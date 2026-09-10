import { OTHER_RUN, RUN, SKILL, SKILL_B, TEST_CASE, VERSION } from "../src/fixtures/platform";

export const ROUTES: [name: string, url: string][] = [
  ["home-empty", "/"],
  ["home-results", "/?q=pdf+%E6%91%98%E8%A6%81"],
  ["compare", `/compare?ids=${SKILL},${SKILL_B}`],
  ["policy", "/policy"],
  ["skill-detail", `/skills/${SKILL}`],
  ["skill-files", `/skills/${SKILL}/files`],
  ["packaging", `/skills/${SKILL}/package?version=${VERSION}`],
  ["lab-run", `/lab/run?skill=${SKILL}&version=${VERSION}&test_case=${TEST_CASE}`],
  ["lab-datasets", `/lab/datasets?test_case=${TEST_CASE}`],
  ["lab-test-cases", "/lab/test-cases"],
  ["lab-test-case-detail", `/lab/test-cases/${TEST_CASE}`],
  ["run-trace", `/runs/${RUN}`],
  ["run-compare", `/runs/${RUN}/compare?against=${OTHER_RUN}`],
  ["workspace-account", "/workspace/account"],
  ["workspace-downloads", "/workspace/downloads"],
  ["workspace-creations", "/workspace/creations"],
  ["workspace-import", "/workspace/import"],
  ["workspace-runs", "/workspace/runs"],
  ["workspace-skills", "/workspace/skills"],
];

export const PHONE_ROUTES = ROUTES;
