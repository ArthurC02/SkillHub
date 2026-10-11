export type CheckNode = {
  kind: string;
  values?: string[];
  children?: CheckNode[];
  at_least?: number;
};

export type Bounds = { min: number; max: number };

export type CheckKind = {
  kind: string;
  category: "grader" | "composite";
  label: string;
  description: string;
  values: Bounds;
  children: Bounds;
  at_least: boolean;
};

export type CheckLimits = { maxNodes: number; maxDepth: number };

export type Path = number[];

export const PROMPT_GRADER = "prompt_grader";

export function pathKey(path: Path): string {
  return ["0", ...path].join(".");
}

export function nodeAt(tree: CheckNode, path: Path): CheckNode {
  return path.reduce<CheckNode>((node, i) => node.children![i], tree);
}

export function replaceAt(tree: CheckNode, path: Path, next: CheckNode): CheckNode {
  if (path.length === 0) return next;
  const [head, ...rest] = path;
  const children = [...(tree.children ?? [])];
  children[head] = replaceAt(children[head], rest, next);
  return { ...tree, children };
}

export function removeAt(tree: CheckNode, path: Path): CheckNode | null {
  if (path.length === 0) return null;
  const parentPath = path.slice(0, -1);
  const parent = nodeAt(tree, parentPath);
  const children = (parent.children ?? []).filter((_, i) => i !== path[path.length - 1]);
  return replaceAt(tree, parentPath, { ...parent, children });
}

export function emptyNode(kind: CheckKind): CheckNode {
  const node: CheckNode = { kind: kind.kind };
  if (kind.values.min > 0) node.values = Array.from({ length: kind.values.min }, () => "");
  if (kind.category === "composite") node.children = [];
  return node;
}

export function appendChild(tree: CheckNode, path: Path, child: CheckNode): CheckNode {
  const parent = nodeAt(tree, path);
  return replaceAt(tree, path, { ...parent, children: [...(parent.children ?? []), child] });
}

export function countNodes(node: CheckNode): number {
  return 1 + (node.children ?? []).reduce((sum, c) => sum + countNodes(c), 0);
}

function depthOf(node: CheckNode): number {
  return 1 + Math.max(0, ...(node.children ?? []).map(depthOf));
}

function nodeProblems(node: CheckNode, kind: CheckKind | undefined): string[] {
  if (!kind) return [`平台不認得「${node.kind}」這種節點`];
  const out: string[] = [];
  const values = node.values ?? [];
  const children = node.children ?? [];
  if (values.length < kind.values.min || values.length > kind.values.max) {
    out.push(
      kind.values.max === 0
        ? "這種節點不帶值"
        : `要 ${kind.values.min} 到 ${kind.values.max} 個值，現在 ${values.length} 個`,
    );
  }
  if (values.some((v) => v.trim() === "")) out.push("有空白的值");
  if (children.length < kind.children.min || children.length > kind.children.max) {
    out.push(
      kind.children.max === 0
        ? "這種節點不能有子節點"
        : `要 ${kind.children.min} 到 ${kind.children.max} 個子節點，現在 ${children.length} 個`,
    );
  }
  const atLeast = node.at_least ?? 0;
  if (kind.at_least && (atLeast < 0 || atLeast > values.length)) {
    out.push(`至少通過數要在 0 到 ${values.length} 之間`);
  }
  return out;
}

export function problems(
  tree: CheckNode,
  kinds: CheckKind[],
  limits: CheckLimits,
): Map<string, string[]> {
  const byKind = new Map(kinds.map((k) => [k.kind, k]));
  const out = new Map<string, string[]>();
  const visit = (node: CheckNode, path: Path) => {
    const found = nodeProblems(node, byKind.get(node.kind));
    if (found.length > 0) out.set(pathKey(path), found);
    (node.children ?? []).forEach((child, i) => visit(child, [...path, i]));
  };
  visit(tree, []);
  const whole: string[] = [];
  if (countNodes(tree) > limits.maxNodes) whole.push(`一個檢查最多 ${limits.maxNodes} 個節點`);
  if (depthOf(tree) > limits.maxDepth) whole.push(`一個檢查最多 ${limits.maxDepth} 層`);
  if (whole.length > 0) out.set("tree", whole);
  return out;
}

export function forRequest(node: CheckNode): CheckNode {
  const out: CheckNode = { kind: node.kind };
  const values = (node.values ?? []).map((v) => v.trim());
  if (values.length > 0) out.values = values;
  if (node.children && node.children.length > 0) out.children = node.children.map(forRequest);
  if (node.at_least) out.at_least = node.at_least;
  return out;
}

export type PlacedNode = { path: Path; column: number; row: number };

export function placeNodes(tree: CheckNode): PlacedNode[] {
  const placed: PlacedNode[] = [];
  let nextRow = 0;
  const place = (node: CheckNode, path: Path): number => {
    const children = node.children ?? [];
    const rows = children.map((child, i) => place(child, [...path, i]));
    const row = rows.length === 0 ? nextRow++ : (rows[0] + rows[rows.length - 1]) / 2;
    placed.push({ path, column: path.length, row });
    return row;
  };
  place(tree, []);
  return placed.sort((a, b) => pathKey(a.path).localeCompare(pathKey(b.path)));
}
