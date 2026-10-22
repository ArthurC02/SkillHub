import { useEffect } from "react";
import {
  Position,
  ReactFlow,
  ReactFlowProvider,
  getNodesBounds,
  useReactFlow,
  useStore,
  type Edge,
  type Node,
  type Rect,
} from "@xyflow/react";
import "@xyflow/react/dist/base.css";
import { pathKey, placeNodes, type CheckNode, type Path } from "../check.model";

const COLUMN_WIDTH = 240;
const ROW_HEIGHT = 72;
const NODE_WIDTH = 200;
const NODE_HEIGHT = 48;

function CanvasTools({ topology, bounds }: { topology: string; bounds: Rect }) {
  const { x, y, width: contentWidth, height: contentHeight } = bounds;
  const width = useStore((state) => state.width);
  const height = useStore((state) => state.height);
  const { fitBounds, viewportInitialized } = useReactFlow();
  useEffect(() => {
    if (viewportInitialized && width > 0 && height > 0)
      void fitBounds({ x, y, width: contentWidth, height: contentHeight }, { padding: 0.2 });
  }, [viewportInitialized, topology, width, height, x, y, contentWidth, contentHeight, fitBounds]);
  return (
    <div className="check-flow-toolbar">
      <button type="button" onClick={() => void fitBounds(bounds, { padding: 0.2 })}>
        顯示全部節點
      </button>
    </div>
  );
}

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
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
      data: { label: <span className="check-flow-label">{describe(p.path)}</span> },
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
    <ReactFlowProvider>
      <div className="check-flow">
        <CanvasTools
          topology={nodes.map((node) => node.id).join("|")}
          bounds={getNodesBounds(nodes)}
        />
        <div className="check-flow-canvas">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            minZoom={0.05}
            preventScrolling={false}
            nodesDraggable={false}
            nodesConnectable={false}
            nodesFocusable={false}
            edgesFocusable={false}
            onNodeClick={(_, node) => onSelect(byKey.get(node.id) ?? [])}
          />
        </div>
      </div>
    </ReactFlowProvider>
  );
}
