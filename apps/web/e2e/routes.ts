import { OTHER_RUN, RUN, SKILL, SKILL_B, TEST_CASE, VERSION } from "../src/fixtures/platform";

/**
 * Every address router.tsx declares, with ids the shared fixtures answer to.
 *
 * One table, used by both the screenshot pass and the phone-width assertion, so
 * a route cannot be pictured without also being checked — or checked without
 * anyone being able to look at what failed.
 */
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
  ["workspace-import", "/workspace/import"],
  ["workspace-runs", "/workspace/runs"],
  ["workspace-skills", "/workspace/skills"],
];

/** The same list; named separately so the assertion reads as what it is. */
export const PHONE_ROUTES = ROUTES;
