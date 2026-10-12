import { CriterionCheckValuesItemMaxLength } from "@skillhub/api-client-ts/limits";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { Loading } from "../../../shared/ui/Loading";
import { ReadFailure } from "../../../shared/ui/LoginRequired";
import {
  PROMPT_GRADER,
  appendChild,
  emptyNode,
  nodeAt,
  pathKey,
  problems,
  removeAt,
  replaceAt,
  type CheckKind,
  type CheckNode,
  type Path,
} from "../check.model";
import { useCheckKinds, useGraders, type Grader } from "../evaluation.service";
import { CheckFlow } from "./CheckFlow";
import { KindPicker } from "./KindPicker";
import "./CheckEditor.css";

type Palette = { kinds: CheckKind[]; byKind: Map<string, CheckKind>; graders: Grader[] };

function describe(node: CheckNode, palette: Palette): string {
  const label = palette.byKind.get(node.kind)?.label ?? node.kind;
  const values = node.values ?? [];
  if (node.kind === PROMPT_GRADER) {
    const grader = palette.graders.find((g) => g.version_id === values[0]);
    return `${label}：${grader ? `${grader.name} v${grader.version}` : "還沒選評分器"}`;
  }
  const shown = values.filter((v) => v.trim() !== "").join("、");
  const atLeast = node.at_least ? `（至少 ${node.at_least} 個）` : "";
  return shown ? `${label}：${shown}${atLeast}` : label;
}

function ValuesField({
  node,
  kind,
  graders,
  idBase,
  onChange,
}: {
  node: CheckNode;
  kind: CheckKind;
  graders: Grader[];
  idBase: string;
  onChange: (values: string[]) => void;
}) {
  const values = node.values ?? [];
  if (node.kind === PROMPT_GRADER) {
    return (
      <p>
        <label htmlFor={`${idBase}-grader`}>評分器版本</label>{" "}
        <select
          id={`${idBase}-grader`}
          value={values[0] ?? ""}
          onChange={(e) => onChange([e.target.value])}
        >
          <option value="">選一個評分器</option>
          {graders.map((g) => (
            <option key={g.version_id} value={g.version_id}>
              {g.name} v{g.version}（{g.scope === "platform" ? "平台通用" : "這個 Workspace"}）
            </option>
          ))}
        </select>{" "}
        <Link to="/lab/graders">管理評分器</Link>
      </p>
    );
  }
  return (
    <fieldset className="check-editor-values">
      <legend>
        值（{kind.values.min}–{kind.values.max} 個）
      </legend>
      {values.map((v, i) => (
        <p key={i}>
          <label htmlFor={`${idBase}-value-${i}`}>第 {i + 1} 個</label>{" "}
          <input
            id={`${idBase}-value-${i}`}
            value={v}
            maxLength={CriterionCheckValuesItemMaxLength}
            onChange={(e) => onChange(values.map((old, j) => (j === i ? e.target.value : old)))}
          />{" "}
          <button
            type="button"
            disabled={values.length <= kind.values.min}
            onClick={() => onChange(values.filter((_, j) => j !== i))}
          >
            刪掉第 {i + 1} 個
          </button>
        </p>
      ))}
      <button
        type="button"
        disabled={values.length >= kind.values.max}
        onClick={() => onChange([...values, ""])}
      >
        加一個值
      </button>
    </fieldset>
  );
}

function NodeForm({
  tree,
  path,
  palette,
  found,
  onChange,
  onSelect,
}: {
  tree: CheckNode;
  path: Path;
  palette: Palette;
  found: string[];
  onChange: (next: CheckNode | null) => void;
  onSelect: (path: Path) => void;
}) {
  const node = nodeAt(tree, path);
  const kind = palette.byKind.get(node.kind);
  const key = pathKey(path);
  const idBase = `check-node-${key.replaceAll(".", "-")}`;
  const update = (patch: Partial<CheckNode>) =>
    onChange(replaceAt(tree, path, { ...node, ...patch }));
  const children = node.children ?? [];
  return (
    <section className="check-editor-node" aria-labelledby={`${idBase}-heading`}>
      <h4 id={`${idBase}-heading`}>選取的節點：{describe(node, palette)}</h4>
      {kind && <p className="note">{kind.description}</p>}
      {kind && kind.values.max > 0 && (
        <ValuesField
          node={node}
          kind={kind}
          graders={palette.graders}
          idBase={idBase}
          onChange={(values) => update({ values })}
        />
      )}
      {kind?.at_least && (
        <p>
          <label htmlFor={`${idBase}-at-least`}>至少幾個值要通過（0 表示全部）</label>{" "}
          <input
            id={`${idBase}-at-least`}
            type="number"
            min={0}
            max={(node.values ?? []).length}
            step={1}
            value={node.at_least ?? 0}
            onChange={(e) => update({ at_least: Number(e.target.value) })}
          />
        </p>
      )}
      {kind?.category === "composite" && children.length < kind.children.max && (
        <KindPicker
          id={`${idBase}-add`}
          label="加一個子節點"
          kinds={palette.kinds}
          action="加入"
          onPick={(picked) => {
            onChange(appendChild(tree, path, emptyNode(picked)));
            onSelect([...path, children.length]);
          }}
        />
      )}
      {found.length > 0 && (
        <ul className="check-editor-problems" aria-label="這個節點還要修正">
          {found.map((p) => (
            <li key={p}>{p}</li>
          ))}
        </ul>
      )}
      <p>
        <button
          type="button"
          onClick={() => {
            onChange(removeAt(tree, path));
            onSelect(path.slice(0, -1));
          }}
        >
          {path.length === 0 ? "拿掉整個檢查" : "刪除這個節點與它底下的節點"}
        </button>
      </p>
    </section>
  );
}

function Outline({
  tree,
  path,
  palette,
  selected,
  flagged,
  onSelect,
}: {
  tree: CheckNode;
  path: Path;
  palette: Palette;
  selected: string;
  flagged: Set<string>;
  onSelect: (path: Path) => void;
}) {
  const node = nodeAt(tree, path);
  const key = pathKey(path);
  return (
    <li>
      <button
        type="button"
        className="check-editor-outline-node"
        aria-current={key === selected ? "true" : undefined}
        onClick={() => onSelect(path)}
      >
        {describe(node, palette)}
        {flagged.has(key) && "（要修正）"}
      </button>
      {(node.children ?? []).length > 0 && (
        <ol>
          {(node.children ?? []).map((_, i) => (
            <Outline
              key={i}
              tree={tree}
              path={[...path, i]}
              palette={palette}
              selected={selected}
              flagged={flagged}
              onSelect={onSelect}
            />
          ))}
        </ol>
      )}
    </li>
  );
}

export function CheckEditor({
  value,
  onChange,
}: {
  value: CheckNode;
  onChange: (next: CheckNode | null) => void;
}) {
  const kinds = useCheckKinds();
  const graders = useGraders("workspace");
  const [selected, setSelected] = useState<Path>([]);

  if (kinds.isPending) return <Loading what="檢查方式" />;
  if (!kinds.data) return <ReadFailure error={kinds.error} what="檢查方式" />;

  const palette: Palette = {
    kinds: kinds.data.kinds,
    byKind: new Map(kinds.data.kinds.map((k) => [k.kind, k])),
    graders: graders.data?.graders ?? [],
  };

  const found = problems(value, palette.kinds, {
    maxNodes: kinds.data.max_nodes,
    maxDepth: kinds.data.max_depth,
  });
  const flagged = new Set(found.keys());
  const current = isPathIn(value, selected) ? selected : [];
  const describeAt = (path: Path) => describe(nodeAt(value, path), palette);

  return (
    <div className="check-editor">
      <CheckFlow
        tree={value}
        describe={describeAt}
        selected={current}
        flagged={flagged}
        onSelect={setSelected}
      />
      <div className="check-editor-panel">
        <ol className="check-editor-outline" aria-label="檢查的結構">
          <Outline
            tree={value}
            path={[]}
            palette={palette}
            selected={pathKey(current)}
            flagged={flagged}
            onSelect={setSelected}
          />
        </ol>
        {found.get("tree")?.map((p) => (
          <p key={p} className="note">
            {p}
          </p>
        ))}
        <NodeForm
          tree={value}
          path={current}
          palette={palette}
          found={found.get(pathKey(current)) ?? []}
          onChange={onChange}
          onSelect={setSelected}
        />
        <ReadFailure error={graders.error} what="評分器" />
      </div>
    </div>
  );
}

function isPathIn(tree: CheckNode, path: Path): boolean {
  let node: CheckNode | undefined = tree;
  for (const i of path) {
    node = node?.children?.[i];
    if (!node) return false;
  }
  return true;
}
