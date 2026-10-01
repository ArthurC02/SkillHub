import { expect, test } from "vitest";
import type { CreationSession, CreationSnapshot, CreationState } from "../creation.service";
import { creationFocus, creationJourney } from "./create.model";

function session(
  state: CreationState = "waiting_input",
  snapshot: Partial<CreationSnapshot> = {},
): CreationSession {
  return {
    id: "session-1",
    revision: 1,
    state,
    snapshot: {
      messages: [],
      brief: "",
      brief_confirmed: false,
      acceptance_criteria: [],
      diagram_understanding: "",
      diagram_confirmed: false,
      references: [],
      pending_action: "",
      budget_credits: 500,
      reserved_credits: 0,
      usage_unknown: false,
      steps: 0,
      tool_calls: 0,
      ...snapshot,
    },
    created_at: "2026-09-29T00:00:00Z",
    updated_at: "2026-09-29T00:00:00Z",
    expires_at: "2026-09-30T00:00:00Z",
    deadline: "2026-09-29T01:00:00Z",
  };
}

test.each([
  ["confirm_brief", "creation-brief-decision"],
  ["confirm_diagram", "creation-diagram-decision"],
  ["answer_diagram_uncertainties", "creation-diagram-decision"],
  ["confirm_diagram_interpretation", "creation-diagram-decision"],
  ["confirm_fetch", "creation-fetch-decision"],
  ["confirm_references", "creation-references-decision"],
  ["confirm_duplicate", "creation-duplicate-decision"],
])("pending action %s points to its evidence card", (pendingAction, target) => {
  const focus = creationFocus(session("waiting_confirmation", { pending_action: pendingAction }));

  expect(focus?.target).toBe(target);
  expect(focus?.title).toBeTruthy();
  expect(focus?.description).toBeTruthy();
});

test("a reupload block and an open reply both point to the composer", () => {
  expect(creationFocus(session("needs_reupload"))?.target).toBe("creation-message");
  expect(creationFocus(session("waiting_input"))?.target).toBe("creation-message");
});

test("a reviewable draft points to the draft while terminal and unknown states invent no decision", () => {
  const draft = {
    revision: 1,
    content_hash: "hash",
    skill: {
      name: "小工具",
      description: "Description",
      compatibility: "",
      allowed_tools: "",
      body: "Body",
      files: null,
    },
    validation: "valid",
    blocked: false,
  };

  expect(creationFocus(session("draft_ready", { draft }))?.target).toBe("creation-draft-decision");
  expect(creationFocus(session("saved", { draft }))).toBeUndefined();
  expect(
    creationFocus(session("waiting_confirmation", { pending_action: "future_action" })),
  ).toBeUndefined();
  expect(creationFocus(session("working"))).toBeUndefined();
});

test.each([
  [undefined, ["current", "upcoming", "upcoming", "upcoming"]],
  [session("waiting_input"), ["current", "upcoming", "upcoming", "upcoming"]],
  [
    session("waiting_confirmation", { brief: "Brief", pending_action: "confirm_brief" }),
    ["complete", "current", "upcoming", "upcoming"],
  ],
  [
    session("waiting_input", { brief: "Brief", brief_confirmed: true }),
    ["complete", "complete", "current", "upcoming"],
  ],
  [
    session("draft_ready", {
      brief: "Brief",
      brief_confirmed: true,
      draft: {
        revision: 1,
        content_hash: "hash",
        skill: {
          name: "小工具",
          description: "Description",
          compatibility: "",
          allowed_tools: "",
          body: "Body",
          files: null,
        },
        validation: "valid",
        blocked: false,
      },
    }),
    ["complete", "complete", "complete", "current"],
  ],
  [
    session("saved", {
      brief: "Brief",
      brief_confirmed: true,
      draft: {
        revision: 1,
        content_hash: "hash",
        skill: {
          name: "小工具",
          description: "Description",
          compatibility: "",
          allowed_tools: "",
          body: "Body",
          files: null,
        },
        validation: "valid",
        blocked: false,
      },
    }),
    ["complete", "complete", "complete", "complete"],
  ],
])("the journey reflects only persisted milestone facts", (value, expected) => {
  expect(creationJourney(value).map((item) => item.status)).toEqual(expected);
});
