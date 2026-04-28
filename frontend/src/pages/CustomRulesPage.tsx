import { useEffect, useState } from 'react';
import { customRulesAPI, CustomRule } from '../api/client';
import { PlayIcon } from '@heroicons/react/24/outline';

export default function CustomRulesPage() {
  const [rules, setRules] = useState<CustomRule[]>([]);
  const [showEditor, setShowEditor] = useState(false);
  const [editingRule, setEditingRule] = useState<CustomRule | null>(null);
  const [yamlCode, setYamlCode] = useState('');
  const [ruleName, setRuleName] = useState('');
  const [ruleDescription, setRuleDescription] = useState('');
  const [severity, setSeverity] = useState('medium');
  const [loading, setLoading] = useState(false);
  const [, setTestResults] = useState<any>(null);

  useEffect(() => {
    loadRules();
  }, []);

  const loadRules = async () => {
    try {
      const result = await customRulesAPI.list();
      setRules(result.rules || []);
    } catch (error) {
      console.error('Failed to load rules:', error);
    }
  };

  const handleCreate = async () => {
    try {
      setLoading(true);
      await customRulesAPI.create({
        name: ruleName,
        description: ruleDescription,
        rule_yaml: yamlCode,
        severity: severity,
      });
      setShowEditor(false);
      setYamlCode('');
      setRuleName('');
      setRuleDescription('');
      loadRules();
    } catch (error: any) {
      alert(`Failed to create rule: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const handleTest = async (ruleId: number) => {
    try {
      setLoading(true);
      const result = await customRulesAPI.test(ruleId);
      setTestResults(result);
      alert(`Rule tested: ${result.match_count} matches found`);
      loadRules();
    } catch (error: any) {
      alert(`Failed to test rule: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const defaultYAML = `name: "Custom Rule"
query: |
  MATCH (u:User)-[:MEMBER_OF|HAS_ROLE*1..3]->(r:Role)
  WHERE r.admin = true
  RETURN u.id as identity_id, u.email, r.name as role_name
severity: high
enabled: true`;

  return (
    <div className="space-y-6">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_30px_rgba(15,23,42,0.9)]">
        <div className="flex justify-between items-center">
          <div>
            <h1 className="text-2xl font-semibold text-slate-50 mb-1">Custom Rules</h1>
            <p className="text-sm text-slate-400">Create and test your own security rules.</p>
          </div>
          <button
            onClick={() => {
              setShowEditor(true);
              setEditingRule(null);
              setYamlCode(defaultYAML);
            }}
            className="px-4 py-2 rounded-lg bg-cyan-600 text-white text-sm font-medium hover:bg-cyan-500"
          >
            + New Rule
          </button>
        </div>
      </div>

      {/* Rule Editor */}
      {showEditor && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
          <h2 className="text-lg font-semibold text-slate-50 mb-4">
            {editingRule ? 'Edit Rule' : 'Create New Rule'}
          </h2>
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-slate-200 mb-1">Rule Name</label>
              <input
                type="text"
                value={ruleName}
                onChange={(e) => setRuleName(e.target.value)}
                className="w-full rounded-lg px-3 py-2 bg-slate-900/80 border border-slate-700 text-slate-100 text-sm"
                placeholder="e.g., AWS Admin with no MFA"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-200 mb-1">Description</label>
              <textarea
                value={ruleDescription}
                onChange={(e) => setRuleDescription(e.target.value)}
                className="w-full rounded-lg px-3 py-2 bg-slate-900/80 border border-slate-700 text-slate-100 text-sm"
                rows={2}
                placeholder="Describe what this rule detects..."
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-200 mb-1">Severity</label>
              <select
                value={severity}
                onChange={(e) => setSeverity(e.target.value)}
                className="rounded-lg px-3 py-2 bg-slate-900/80 border border-slate-700 text-slate-100 text-sm"
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-200 mb-1">Rule YAML</label>
              <textarea
                value={yamlCode}
                onChange={(e) => setYamlCode(e.target.value)}
                className="w-full rounded-lg px-3 py-2 bg-slate-900/80 border border-slate-700 text-slate-100 font-mono text-xs"
                rows={10}
                placeholder={defaultYAML}
              />
            </div>
            <div className="flex space-x-2">
              <button
                onClick={handleCreate}
                disabled={loading || !ruleName || !yamlCode}
                className="px-4 py-2 bg-cyan-600 text-white rounded-lg hover:bg-cyan-500 disabled:opacity-50 text-sm"
              >
                {editingRule ? 'Update' : 'Create'} Rule
              </button>
              <button
                onClick={() => {
                  setShowEditor(false);
                  setYamlCode('');
                }}
                className="px-4 py-2 bg-slate-800 text-slate-200 rounded-lg hover:bg-slate-700 text-sm"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Rules List */}
      <div className="space-y-4">
        {rules.map((rule) => (
          <div
            key={rule.id}
            className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]"
          >
            <div className="flex justify-between items-start mb-4">
              <div>
                <h3 className="text-lg font-semibold text-slate-50">{rule.name}</h3>
                {rule.description && (
                  <p className="text-sm text-slate-400 mt-1">{rule.description}</p>
                )}
              </div>
              <div className="flex items-center space-x-2">
                <span
                  className={`px-2 py-1 text-xs rounded capitalize ${
                    rule.severity === 'critical'
                      ? 'bg-rose-500/20 text-rose-200 border border-rose-400/60'
                      : rule.severity === 'high'
                      ? 'bg-orange-500/20 text-orange-200 border border-orange-400/60'
                      : 'bg-amber-500/20 text-amber-200 border border-amber-400/60'
                  }`}
                >
                  {rule.severity}
                </span>
                <span
                  className={`px-2 py-1 text-xs rounded ${
                    rule.enabled
                      ? 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/60'
                      : 'bg-slate-700/80 text-slate-200 border border-slate-500/60'
                  }`}
                >
                  {rule.enabled ? 'Enabled' : 'Disabled'}
                </span>
              </div>
            </div>

            <div className="bg-slate-900/80 rounded-lg p-4 mb-4 border border-slate-800/80">
              <pre className="text-[11px] text-slate-100 overflow-x-auto whitespace-pre-wrap">
                {rule.rule_yaml}
              </pre>
            </div>

            <div className="flex justify-between items-center">
              <div className="text-xs text-slate-400">
                Created by {rule.created_by} • {new Date(rule.created_at).toLocaleDateString()}
                {rule.last_tested_at && (
                  <span> • Last tested: {new Date(rule.last_tested_at).toLocaleDateString()}</span>
                )}
              </div>
              <button
                onClick={() => handleTest(rule.id)}
                disabled={loading}
                className="px-4 py-2 bg-emerald-600 text-white rounded-lg hover:bg-emerald-500 disabled:opacity-50 flex items-center space-x-2 text-sm"
              >
                <PlayIcon className="h-4 w-4" />
                <span>Test Rule</span>
              </button>
            </div>

            {rule.test_results && (
              <div className="mt-4 p-4 bg-cyan-950/60 rounded-lg border border-cyan-500/60">
                <div className="text-sm font-medium text-cyan-100">Last Test Results:</div>
                <div className="text-xs text-cyan-100/80 mt-1">
                  {rule.test_results.match_count || 0} matches found
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
