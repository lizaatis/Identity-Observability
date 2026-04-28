import { useEffect, useState } from 'react';
import ReactFlow, { Node, Edge, Background, Controls, MiniMap, MarkerType } from 'reactflow';
import 'reactflow/dist/style.css';
import { graphAPI, BlastRadiusResponse } from '../api/client';

interface BlastRadiusProps {
  identityId: number;
}

export default function BlastRadius({ identityId }: BlastRadiusProps) {
  const [data, setData] = useState<BlastRadiusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [maxHops, setMaxHops] = useState(3);

  useEffect(() => {
    const loadData = async () => {
      try {
        setLoading(true);
        const result = await graphAPI.getBlastRadius(identityId, maxHops);
        setData(result);
      } catch (error) {
        console.error('Failed to load blast radius:', error);
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, [identityId, maxHops]);

  if (loading) {
    return <div className="text-center py-12">Loading blast radius...</div>;
  }

  if (!data) {
    return (
      <div className="text-center py-12 text-red-600">
        <p className="font-medium">Failed to load blast radius</p>
        <p className="text-sm text-gray-600 mt-2 max-w-md mx-auto">
          Blast Radius uses the graph database (Neo4j). Start Neo4j with docker compose, then run &quot;Sync Graph&quot; to populate it.
        </p>
      </div>
    );
  }

  // Empty graph with explanation (e.g. Neo4j not connected)
  if (data.nodes.length === 0 && data.message) {
    return (
      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-xl font-semibold mb-4">Blast Radius Analysis</h2>
        <div className="text-center py-12 rounded-lg bg-amber-50 border border-amber-200">
          <p className="text-amber-800 font-medium">{data.message}</p>
        </div>
      </div>
    );
  }

  // Convert to React Flow format
  const nodes: Node[] = data.nodes.map((node) => ({
    id: node.id,
    data: { label: node.label },
    position: { x: Math.random() * 800, y: Math.random() * 600 },
    style: getNodeStyle(node.type),
  }));

  const edges: Edge[] = data.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.label,
    type: 'smoothstep',
    animated: true,
    markerEnd: {
      type: MarkerType.ArrowClosed,
    },
    style: {
      stroke: getEdgeColor(edge.distance),
      strokeWidth: 2,
    },
  }));

  return (
    <div className="space-y-4">
      <div className="bg-white shadow rounded-lg p-6">
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-xl font-semibold">Blast Radius Analysis</h2>
          <div className="flex items-center space-x-4">
            <label className="text-sm">
              Max Hops:
              <select
                value={maxHops}
                onChange={(e) => setMaxHops(Number(e.target.value))}
                className="ml-2 border rounded px-2 py-1"
              >
                {[1, 2, 3, 4, 5].map((h) => (
                  <option key={h} value={h}>
                    {h}
                  </option>
                ))}
              </select>
            </label>
            <button
              onClick={async () => {
                setLoading(true);
                const result = await graphAPI.syncGraph();
                if (result.status === 'success') {
                  const blastRadius = await graphAPI.getBlastRadius(identityId, maxHops);
                  setData(blastRadius);
                }
                setLoading(false);
              }}
              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 text-sm"
            >
              Sync Graph
            </button>
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-4">
          <div className="border rounded p-3">
            <div className="text-sm text-gray-500">Total Nodes</div>
            <div className="text-2xl font-bold">{data.stats.total_nodes}</div>
          </div>
          <div className="border rounded p-3">
            <div className="text-sm text-gray-500">Groups</div>
            <div className="text-2xl font-bold">{data.stats.groups_reachable}</div>
          </div>
          <div className="border rounded p-3">
            <div className="text-sm text-gray-500">Roles</div>
            <div className="text-2xl font-bold">{data.stats.roles_reachable}</div>
          </div>
          <div className="border rounded p-3">
            <div className="text-sm text-gray-500">Permissions</div>
            <div className="text-2xl font-bold">{data.stats.permissions_reachable}</div>
          </div>
          <div className="border rounded p-3">
            <div className="text-sm text-gray-500">Resources</div>
            <div className="text-2xl font-bold">{data.stats.resources_reachable}</div>
          </div>
        </div>
      </div>

      {/* Graph Visualization */}
      <div className="bg-white shadow rounded-lg" style={{ height: '600px' }}>
        <ReactFlow nodes={nodes} edges={edges} fitView>
          <Background />
          <Controls />
          <MiniMap />
        </ReactFlow>
      </div>
    </div>
  );
}

function getNodeStyle(type: string): React.CSSProperties {
  const colors: Record<string, { bg: string; border: string }> = {
    User: { bg: '#3b82f6', border: '#2563eb' },
    Group: { bg: '#10b981', border: '#059669' },
    Role: { bg: '#f59e0b', border: '#d97706' },
    Permission: { bg: '#8b5cf6', border: '#7c3aed' },
    Resource: { bg: '#ec4899', border: '#db2777' },
  };

  const color = colors[type] || { bg: '#6b7280', border: '#4b5563' };

  return {
    background: color.bg,
    color: '#fff',
    border: `2px solid ${color.border}`,
    borderRadius: '8px',
    padding: '10px',
    fontWeight: 'bold',
  };
}

function getEdgeColor(distance: number): string {
  const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'];
  return colors[Math.min(distance - 1, colors.length - 1)] || '#6b7280';
}
