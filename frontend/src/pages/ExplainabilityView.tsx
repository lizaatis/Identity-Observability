import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import ReactFlow, { Node, Edge, Background, Controls, MiniMap } from 'reactflow';
import 'reactflow/dist/style.css';
import { identityAPI, IdentityDetail, PermissionLineage } from '../api/client';

export default function ExplainabilityView() {
  const { id } = useParams<{ id: string }>();
  const [identity, setIdentity] = useState<IdentityDetail | null>(null);
  const [selectedPermission, setSelectedPermission] = useState<PermissionLineage | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;

    const loadData = async () => {
      try {
        const data = await identityAPI.get(Number(id));
        setIdentity(data);
        if (data.lineage.length > 0) {
          setSelectedPermission(data.lineage[0]);
        }
      } catch (error) {
        console.error('Failed to load identity:', error);
      } finally {
        setLoading(false);
      }
    };

    loadData();
  }, [id]);

  if (loading) {
    return <div className="text-center py-12">Loading...</div>;
  }

  if (!identity || !selectedPermission) {
    return <div className="text-center py-12">No data available</div>;
  }

  const { nodes, edges } = buildGraph(identity, selectedPermission);

  return (
    <div className="space-y-6">
      <div className="bg-white shadow rounded-lg p-6">
        <h1 className="text-2xl font-bold mb-4">Access Explainability</h1>
        <div className="mb-4">
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Select Permission
          </label>
          <select
            value={selectedPermission.permission_id}
            onChange={(e) => {
              const perm = identity.lineage.find((p) => p.permission_id === Number(e.target.value));
              if (perm) setSelectedPermission(perm);
            }}
            className="block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          >
            {identity.lineage.map((perm) => (
              <option key={perm.permission_id} value={perm.permission_id}>
                {perm.permission_name}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="bg-white shadow rounded-lg" style={{ height: '600px' }}>
        <ReactFlow nodes={nodes} edges={edges} fitView>
          <Background />
          <Controls />
          <MiniMap />
        </ReactFlow>
      </div>

      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-xl font-semibold mb-4">Path Details</h2>
        <div className="space-y-2">
          {selectedPermission.path.map((hop, idx) => (
            <div
              key={idx}
              className={`flex items-center space-x-4 p-3 rounded ${
                hop.hop_type === 'permission' ? 'bg-blue-50' : 'bg-gray-50'
              }`}
            >
              <span className="text-sm font-medium text-gray-500 w-24">{hop.hop_type}</span>
              <span className="flex-1 font-medium">{hop.hop_name}</span>
              {hop.hop_detail && (
                <span className="text-sm text-gray-500">({hop.hop_detail})</span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function buildGraph(identity: IdentityDetail, permission: PermissionLineage): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  const nodeIds = new Set<string>();

  // Add identity node
  const identityNodeId = 'identity';
  nodes.push({
    id: identityNodeId,
    type: 'input',
    data: { label: identity.identity.display_name || identity.identity.email },
    position: { x: 100, y: 200 },
    style: { background: '#3b82f6', color: '#fff' },
  });
  nodeIds.add(identityNodeId);

  // Add path nodes
  let xOffset = 300;
  let prevNodeId = identityNodeId;

  for (const hop of permission.path) {
    const nodeId = `${hop.hop_type}-${hop.ord}`;
    if (nodeIds.has(nodeId)) continue;

    const isDeadend = hop.hop_type === 'permission' && 
      identity.effective_permissions.some(p => 
        p.permission_id === permission.permission_id && 
        p.path_type === 'via_group'
      );

    nodes.push({
      id: nodeId,
      data: { label: hop.hop_name },
      position: { x: xOffset, y: 200 },
      style: {
        background: isDeadend ? '#ef4444' : '#10b981',
        color: '#fff',
      },
    });
    nodeIds.add(nodeId);

    edges.push({
      id: `${prevNodeId}-${nodeId}`,
      source: prevNodeId,
      target: nodeId,
      animated: true,
    });

    prevNodeId = nodeId;
    xOffset += 300;
  }

  return { nodes, edges };
}
