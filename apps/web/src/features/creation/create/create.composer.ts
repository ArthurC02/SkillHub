import { useEffect, useMemo, useRef, useState } from "react";
import type { CreationSession } from "../creation.service";
import type { CreationCommands } from "./create.commands";
import {
  compositionCommand,
  compositionMode,
  compositionProblem,
  diagramProblem,
  readImage,
} from "./create.model";

type Reference = { id: string; name: string };

export function useComposer(budget: string, onProblem: (error: Error | undefined) => void) {
  const [picking, setPicking] = useState(false),
    [dragging, setDragging] = useState(false),
    [message, setMessage] = useState(""),
    [file, setFile] = useState<File>(),
    [refs, setRefs] = useState<Reference[]>([]);
  const fileInput = useRef<HTMLInputElement>(null);
  const textarea = useRef<HTMLTextAreaElement>(null);
  useEffect(() => textarea.current?.focus(), [budget]);
  const clearFile = () => {
    setFile(undefined);
    if (fileInput.current) fileInput.current.value = "";
  };
  const chooseFile = (picked?: File) => {
    if (!picked) return;
    const problem = diagramProblem(picked);
    if (problem) {
      clearFile();
      onProblem(new Error(problem));
      return;
    }
    onProblem(undefined);
    setFile(picked);
  };
  const [thumbs] = useState(() => new Map<string, string>());
  useEffect(() => () => thumbs.forEach((url) => URL.revokeObjectURL(url)), [thumbs]);
  const preview = useMemo(() => (file ? URL.createObjectURL(file) : undefined), [file]);
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview);
    },
    [preview],
  );

  const send = async (
    commands: CreationCommands,
    session: CreationSession | undefined,
    budgetCredits: number | undefined,
  ) => {
    const problem = compositionProblem(message.trim(), !!file, refs.length);
    if (problem) throw new Error(problem);
    const mode = compositionMode(file, refs.length);
    const diagram = mode === "diagram" && file ? await readImage(file) : undefined;
    if (mode === "diagram" && !diagram) throw new Error("請先選擇流程圖。");
    let value = session;
    if (!value) {
      if (budgetCredits === undefined) return;
      value = await commands.start(mode === "message" ? message : "", budgetCredits);
      if (mode === "message") {
        setMessage("");
        return;
      }
    }
    const next = await commands.send(
      value,
      ...compositionCommand(
        message,
        diagram,
        refs.map((r) => r.id),
      ),
    );
    if (mode === "diagram") {
      const recorded = next.snapshot.attachments ?? [];
      const mine = recorded[recorded.length - 1];
      if (mine && file) thumbs.set(mine.sha256, URL.createObjectURL(file));
      clearFile();
    }
    if (mode === "references") setRefs([]);
    setMessage("");
  };

  return {
    inputs: {
      picking,
      onPicking: setPicking,
      dragging,
      onDragging: setDragging,
      message,
      onMessage: setMessage,
      file,
      preview,
      onChooseFile: chooseFile,
      onClearFile: clearFile,
      fileInput,
      refs,
      onRefs: setRefs,
      textarea,
      hasContent: message.trim() !== "" || !!file || refs.length > 0,
    },
    thumbs,
    setMessage,
    send,
    startFrom: (prompt: string) => {
      setMessage(prompt);
      textarea.current?.focus();
    },
    reset: () => {
      setMessage("");
      clearFile();
      setRefs([]);
    },
  };
}
