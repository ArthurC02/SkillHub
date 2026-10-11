import { Position, ReactFlow, type Edge, type Node } from "@xyflow/react";
import "@xyflow/react/dist/base.css";
import { pathKey, placeNodes, type CheckNode, type Path } from "../check.model";

const COLUMN_WIDTH = 240;
const ROW_HEIGHT = 72;

export function CheckFlow({
  tree,
  describe,
  selected,
  flagged,
  onSelect,
}: {
  tree: CheckNode;
  describe: (path: Path) => string;
  selected: Path;
  flagged: Set<string>;
  onSelect: (path: Path) => void;
}) {
  const placed = placeNodes(tree);
  const byKey = new Map(placed.map((p) => [pathKey(p.path), p.path]));
  const selectedKey = pathKey(selected);
  const nodes: Node[] = placed.map((p) => {
    const key = pathKey(p.path);
    return {
      id: key,
      position: { x: p.column * COLUMN_WIDTH, y: p.row * ROW_HEIGHT },
      data: { label: describe(p.path) },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      selected: key === selectedKey,
      className: flagged.has(key) ? "check-flow-node check-flow-flagged" : "check-flow-node",
    };
  });
  const edges: Edge[] = placed
    .filter((p) => p.path.length > 0)
    .map((p) => {
      const target = pathKey(p.path);
      const source = pathKey(p.path.slice(0, -1));
      return { id: `${source}-${target}`, source, target };
    });
  return (
    <div className="check-flow">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        onNodeClick={(_, node) => onSelect(byKey.get(node.id) ?? [])}
      />
    </div>
  );
}
