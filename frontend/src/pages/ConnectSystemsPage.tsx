import { useState } from 'react';
import { Link } from 'react-router-dom';
import { connectorAPI } from '../api/client';
import { formatSourceSystem } from '../utils/formatSourceSystem';

type ConnectorType = 'okta' | 'sailpoint' | 'gcp';

export default function ConnectSystemsPage() {
  const [oktaDomain, setOktaDomain] = useState('');
  const [oktaToken, setOktaToken] = useState('');
  const [oktaTestResult, setOktaTestResult] = useState<{ ok: boolean; message?: string; error?: string } | null>(null);
  const [sailpointTenant, setSailpointTenant] = useState('');
  const [sailpointClientId, setSailpointClientId] = useState('');
  const [sailpointSecret, setSailpointSecret] = useState('');
  const [sailpointTestResult, setSailpointTestResult] = useState<{ ok: boolean; message?: string; error?: string } | null>(null);
  const [gcpProjectId, setGcpProjectId] = useState('');
  const [gcpTestResult, setGcpTestResult] = useState<{ ok: boolean; message?: string; error?: string } | null>(null);
  const [testing, setTesting] = useState<ConnectorType | null>(null);

  const testOkta = async () => {
    setTesting('okta');
    setOktaTestResult(null);
    try {
      const res = await connectorAPI.testOkta(oktaDomain.trim(), oktaToken);
      setOktaTestResult(res);
    } catch (e: any) {
      setOktaTestResult({ ok: false, error: e.message || 'Request failed' });
    } finally {
      setTesting(null);
    }
  };

  const testSailPoint = async () => {
    setTesting('sailpoint');
    setSailpointTestResult(null);
    try {
      const res = await connectorAPI.testSailPoint(sailpointTenant.trim(), sailpointClientId, sailpointSecret);
      setSailpointTestResult(res);
    } catch (e: any) {
      setSailpointTestResult({ ok: false, error: e.message || 'Request failed' });
    } finally {
      setTesting(null);
    }
  };

  const testGCP = async () => {
    setTesting('gcp');
    setGcpTestResult(null);
    try {
      const res = await connectorAPI.testGCP(gcpProjectId.trim());
      setGcpTestResult(res);
    } catch (e: any) {
      setGcpTestResult({ ok: false, error: e.message || 'Request failed' });
    } finally {
      setTesting(null);
    }
  };

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-semibold text-slate-50">Connect Systems</h1>
        <p className="text-sm text-slate-400 mt-1">
          Add Okta, SailPoint, and GCP in under 30 minutes. Test credentials below, then run connectors via CLI. View status in{' '}
          <Link to="/connectors" className="text-cyan-400 hover:underline">Connector Health</Link>.
        </p>
      </div>

      <div className="grid gap-6 max-w-2xl">
        {/* Okta */}
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6">
          <h2 className="text-lg font-medium text-slate-50">{formatSourceSystem('okta_primary')}</h2>
          <p className="text-xs text-slate-400 mt-1">URL + API token (Admin API token with read users/groups)</p>
          <div className="mt-4 space-y-3">
            <input
              type="url"
              placeholder="https://your-org.okta.com"
              value={oktaDomain}
              onChange={(e) => setOktaDomain(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <input
              type="password"
              placeholder="API token"
              value={oktaToken}
              onChange={(e) => setOktaToken(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <button
              onClick={testOkta}
              disabled={testing !== null || !oktaDomain.trim() || !oktaToken}
              className="px-4 py-2 rounded-lg bg-cyan-600/80 text-white text-sm font-medium hover:bg-cyan-500 disabled:opacity-50"
            >
              {testing === 'okta' ? 'Testing…' : 'Test connection'}
            </button>
            {oktaTestResult && (
              <div className={`text-sm ${oktaTestResult.ok ? 'text-emerald-400' : 'text-rose-400'}`}>
                {oktaTestResult.ok ? (oktaTestResult.message || 'OK') : (oktaTestResult.error || 'Failed')}
              </div>
            )}
          </div>
        </div>

        {/* SailPoint */}
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6">
          <h2 className="text-lg font-medium text-slate-50">{formatSourceSystem('sailpoint_identitynow')}</h2>
          <p className="text-xs text-slate-400 mt-1">Tenant URL + client ID and secret</p>
          <div className="mt-4 space-y-3">
            <input
              type="url"
              placeholder="https://your-tenant.api.identitynow.com"
              value={sailpointTenant}
              onChange={(e) => setSailpointTenant(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <input
              type="text"
              placeholder="Client ID"
              value={sailpointClientId}
              onChange={(e) => setSailpointClientId(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <input
              type="password"
              placeholder="Client secret"
              value={sailpointSecret}
              onChange={(e) => setSailpointSecret(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <button
              onClick={testSailPoint}
              disabled={testing !== null || !sailpointTenant.trim() || !sailpointClientId || !sailpointSecret}
              className="px-4 py-2 rounded-lg bg-cyan-600/80 text-white text-sm font-medium hover:bg-cyan-500 disabled:opacity-50"
            >
              {testing === 'sailpoint' ? 'Testing…' : 'Test connection'}
            </button>
            {sailpointTestResult && (
              <div className={`text-sm ${sailpointTestResult.ok ? 'text-emerald-400' : 'text-rose-400'}`}>
                {sailpointTestResult.ok ? (sailpointTestResult.message || 'OK') : (sailpointTestResult.error || 'Failed')}
              </div>
            )}
          </div>
        </div>

        {/* GCP */}
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6">
          <h2 className="text-lg font-medium text-slate-50">{formatSourceSystem('gcp_iam')}</h2>
          <p className="text-xs text-slate-400 mt-1">Project ID. Set GOOGLE_APPLICATION_CREDENTIALS to your service account JSON path when running the connector.</p>
          <div className="mt-4 space-y-3">
            <input
              type="text"
              placeholder="my-gcp-project-id"
              value={gcpProjectId}
              onChange={(e) => setGcpProjectId(e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-slate-800/80 border border-slate-600 text-slate-200 text-sm"
            />
            <button
              onClick={testGCP}
              disabled={testing !== null || !gcpProjectId.trim()}
              className="px-4 py-2 rounded-lg bg-cyan-600/80 text-white text-sm font-medium hover:bg-cyan-500 disabled:opacity-50"
            >
              {testing === 'gcp' ? 'Testing…' : 'Test connection'}
            </button>
            {gcpTestResult && (
              <div className={`text-sm ${gcpTestResult.ok ? 'text-emerald-400' : 'text-rose-400'}`}>
                {gcpTestResult.ok ? (gcpTestResult.message || 'OK') : (gcpTestResult.error || 'Failed')}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="text-sm text-slate-400">
        After testing, run connectors from the project directory (e.g. <code className="bg-slate-800 px-1 rounded">go run .</code> in connectors/okta) and check{' '}
        <Link to="/connectors" className="text-cyan-400 hover:underline">Connector Health</Link> for last sync status and errors.
      </div>
    </div>
  );
}
