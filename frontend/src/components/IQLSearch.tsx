import { useState, useEffect } from 'react';
import { graphAPI } from '../api/client';
import { MagnifyingGlassIcon } from '@heroicons/react/24/outline';

export default function IQLSearch() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Array<Record<string, any>>>([]);
  const [loading, setLoading] = useState(false);
  const [fields, setFields] = useState<string[]>([]);
  const [examples, setExamples] = useState<string[]>([]);

  useEffect(() => {
    const loadFields = async () => {
      try {
        const data = await graphAPI.getIQLFields();
        setFields(data.fields || []);
        setExamples(data.examples || []);
      } catch (error) {
        console.error('Failed to load IQL fields:', error);
      }
    };
    loadFields();
  }, []);

  const handleSearch = async () => {
    if (!query.trim()) return;

    try {
      setLoading(true);
      const result = await graphAPI.iqlSearch(query);
      setResults(result.results || []);
    } catch (error) {
      console.error('IQL search failed:', error);
      setResults([]);
    } finally {
      setLoading(false);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleSearch();
    }
  };

  return (
    <div className="space-y-4">
      <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
        <h2 className="text-lg font-semibold mb-2 text-slate-50">Identity Query Language (IQL)</h2>

        {/* Search Bar */}
        <div className="flex space-x-2 mb-4">
          <div className="flex-1 relative">
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder="system:aws mfa:false"
              className="w-full rounded-lg px-4 py-2 pl-10 bg-slate-900/80 border border-slate-700 text-slate-100 focus:ring-2 focus:ring-cyan-500 focus:border-cyan-500 text-sm"
            />
            <MagnifyingGlassIcon className="absolute left-3 top-3 h-5 w-5 text-slate-500" />
          </div>
          <button
            onClick={handleSearch}
            disabled={loading || !query.trim()}
            className="px-6 py-2 bg-cyan-600 text-white rounded-lg hover:bg-cyan-500 disabled:opacity-50 disabled:cursor-not-allowed text-sm"
          >
            {loading ? 'Searching...' : 'Search'}
          </button>
        </div>

        {/* Supported Fields */}
        <div className="mb-4">
          <h3 className="text-sm font-medium text-slate-200 mb-2">Supported Fields:</h3>
          <div className="flex flex-wrap gap-2">
            {fields.map((field) => (
              <span
                key={field}
                className="px-2 py-1 bg-slate-900/80 text-slate-200 rounded text-[11px] border border-slate-700"
              >
                {field}
              </span>
            ))}
          </div>
        </div>

        {/* Examples */}
        {examples.length > 0 && (
          <div className="mb-4">
            <h3 className="text-sm font-medium text-slate-200 mb-2">Examples:</h3>
            <div className="space-y-1">
              {examples.map((example, idx) => (
                <button
                  key={idx}
                  onClick={async () => {
                    setQuery(example);
                    setLoading(true);
                    try {
                      const result = await graphAPI.iqlSearch(example);
                      setResults(result.results || []);
                    } catch (error) {
                      console.error('IQL search failed:', error);
                      setResults([]);
                    } finally {
                      setLoading(false);
                    }
                  }}
                  className="block text-sm text-cyan-300 hover:text-cyan-200 hover:underline"
                >
                  {example}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Results */}
      {results.length > 0 && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 shadow-[0_0_24px_rgba(15,23,42,0.9)]">
          <h3 className="text-lg font-semibold mb-4 text-slate-50">
            Results ({results.length})
          </h3>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-slate-800">
              <thead className="bg-slate-900/80">
                <tr>
                  {Object.keys(results[0]).map((key) => (
                    <th
                      key={key}
                      className="px-6 py-3 text-left text-[11px] font-medium text-slate-400 uppercase tracking-wider"
                    >
                      {key.replace(/_/g, ' ')}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="bg-slate-950 divide-y divide-slate-800">
                {results.map((result, idx) => (
                  <tr key={idx}>
                    {Object.values(result).map((value, valIdx) => (
                      <td
                        key={valIdx}
                        className="px-6 py-4 whitespace-nowrap text-sm text-slate-100"
                      >
                        {typeof value === 'object' ? JSON.stringify(value) : String(value)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {!loading && query && results.length === 0 && (
        <div className="bg-slate-950/80 border border-slate-800/80 rounded-2xl p-6 text-center text-slate-300 text-sm">
          No results found for query:{' '}
          <code className="bg-slate-900/80 px-2 py-1 rounded border border-slate-700 text-slate-100">
            {query}
          </code>
        </div>
      )}
    </div>
  );
}
