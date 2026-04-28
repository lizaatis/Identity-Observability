import { Link } from 'react-router-dom';

export default function SetupPage() {
  return (
    <div className="space-y-8 max-w-3xl">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <p className="text-[11px] font-semibold text-cyan-300 uppercase tracking-[0.25em]">Runbook</p>
        <h1 className="mt-2 text-2xl font-semibold text-slate-50">Get the platform trusted and visible</h1>
        <p className="mt-2 text-sm text-slate-400">
          Follow these steps in order so dashboards, drift, and graph features are not “empty by accident.”
        </p>
      </div>

      <ol className="space-y-6 list-decimal list-inside text-slate-300 text-sm">
        <li className="pl-2">
          <span className="font-medium text-slate-100">Run database migrations</span>
          <p className="mt-1 text-slate-400 ml-6 list-none">
            From the repository root (not inside <code className="text-cyan-200/90">scripts/</code>), with{' '}
            <code className="text-cyan-200/90">DATABASE_URL</code> pointing at the same Postgres the API uses:
          </p>
          <pre className="mt-2 ml-6 p-4 rounded-xl bg-slate-900/90 border border-slate-700/80 text-xs text-slate-200 overflow-x-auto">
            {`export DATABASE_URL='postgres://observability:observability_dev@localhost:5434/identity_observability?sslmode=disable'
./scripts/run-migrations.sh all`}
          </pre>
          <p className="mt-2 ml-6 text-slate-500 text-xs">
            This applies canonical schema, events (006), automation (007), metadata (008), and platform/graph state (009).
          </p>
        </li>

        <li className="pl-2">
          <span className="font-medium text-slate-100">Run the mock connector (or your real connector)</span>
          <p className="mt-1 text-slate-400 ml-6">
            Seed <code className="text-cyan-200/90">sync_runs</code> and graph-related data so the dashboard is non-zero.
            Use the mock connector README under <code className="text-cyan-200/90">connectors/mock</code>.
          </p>
        </li>

        <li className="pl-2">
          <span className="font-medium text-slate-100">Verify the dashboard and diagnostics</span>
          <p className="mt-1 text-slate-400 ml-6">
            Open the <Link className="text-cyan-300 hover:text-cyan-200" to="/">Risk Dashboard</Link> — counts should reflect
            identities. If something is wrong, the banner on the dashboard explains missing migrations, Neo4j, or event tables.
          </p>
          <p className="mt-2 ml-6 text-slate-400">
            Optional: <code className="text-cyan-200/90">GET /health</code> returns Postgres + Neo4j connectivity for operators.
          </p>
        </li>

        <li className="pl-2">
          <span className="font-medium text-slate-100">Neo4j graph (paths, toxic combos, blast radius)</span>
          <p className="mt-1 text-slate-400 ml-6">
            Start Neo4j (e.g. <code className="text-cyan-200/90">docker compose up -d neo4j</code>), then trigger{' '}
            <strong className="text-slate-200">Sync Graph</strong> from the UI or <code className="text-cyan-200/90">POST /api/v1/graph/sync</code>.
            Check <code className="text-cyan-200/90">GET /api/v1/graph/status</code> for last sync time and node/edge counts.
          </p>
        </li>
      </ol>

      <div className="rounded-2xl border border-slate-700/60 bg-slate-900/40 p-5 text-xs text-slate-400">
        <p className="font-medium text-slate-200 text-sm mb-2">What not to chase before the core loop works</p>
        <ul className="list-disc list-inside space-y-1 text-slate-400">
          <li>Second graph database or full ML-based stitching</li>
          <li>Replacing warehouse analytics — focus on dossier + graph + drift first</li>
        </ul>
      </div>
    </div>
  );
}
