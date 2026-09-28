import { useRef, useState } from "react";
import { ApiError } from "../../../core/api/client";
import {
  actOnCreationSession,
  createCreationSession,
  useCreationSessionCache,
  type CreationAction,
  type CreationSession,
} from "../creation.service";

export type CommandExtra = Omit<CreationAction, "command_id" | "expected_revision" | "kind">;

export type Perform = (kind: CreationAction["kind"], extra?: CommandExtra) => Promise<void>;

type StartBody = { id: string; message: string; budget_credits: number };

const newCommandID = () => {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  return [...bytes]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("")
    .replace(/(.{8})(.{4})(.{4})(.{4})(.{12})/, "$1-$2-$3-$4-$5");
};

export function useCreationCommands(onSaved: (id: string) => void) {
  const cache = useCreationSessionCache();
  const pending = useRef<{ key: string; body: CreationAction } | undefined>(undefined);
  const startPending = useRef<{ key: string; body: StartBody } | undefined>(undefined);
  const save = (value: CreationSession) => {
    onSaved(value.id);
    cache.remember(value);
  };
  const send = async (
    value: CreationSession,
    kind: CreationAction["kind"],
    extra: CommandExtra = {},
  ) => {
    const key = JSON.stringify([value.id, kind, extra]);
    if (pending.current?.key !== key)
      pending.current = {
        key,
        body: {
          command_id: newCommandID(),
          expected_revision: value.revision,
          kind,
          ...extra,
        },
      };
    try {
      const next = await actOnCreationSession(value.id, pending.current.body);
      pending.current = undefined;
      save(next);
      return next;
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        pending.current = undefined;
        await cache.reload(value.id);
      }
      throw err;
    }
  };
  const start = async (message: string, budgetCredits: number) => {
    const key = JSON.stringify([message, budgetCredits]);
    if (startPending.current?.key !== key)
      startPending.current = {
        key,
        body: { id: newCommandID(), message, budget_credits: budgetCredits },
      };
    const value = await createCreationSession(startPending.current.body);
    save(value);
    startPending.current = undefined;
    return value;
  };
  const forgetPending = () => {
    pending.current = undefined;
  };
  return { send, start, forgetPending };
}

export type CreationCommands = ReturnType<typeof useCreationCommands>;

export type Attempt = "submit" | [CreationAction["kind"], CommandExtra];

export function useCreationAttempt() {
  const [error, setError] = useState<unknown>(),
    [busy, setBusy] = useState(false);
  const [lastAttempt, setLastAttempt] = useState<Attempt>();
  const attempt = async (retryAs: Attempt, work: () => Promise<void>) => {
    setBusy(true);
    setError(undefined);
    try {
      await work();
    } catch (err) {
      setError(err);
      setLastAttempt(retryAs);
    } finally {
      setBusy(false);
    }
  };
  return { error, setError, busy, lastAttempt, attempt };
}
