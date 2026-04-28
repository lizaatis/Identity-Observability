import { useState } from 'react';
import { exportAPI } from '../api/client';

interface ExportButtonProps {
  identityId: number;
}

export default function ExportButton({ identityId }: ExportButtonProps) {
  const [exporting, setExporting] = useState(false);
  const [showMenu, setShowMenu] = useState(false);

  const handleExport = async (format: 'csv' | 'pdf') => {
    try {
      setExporting(true);
      setShowMenu(false);
      const blob = await exportAPI.exportIdentity(identityId, format);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `identity-${identityId}-${new Date().toISOString()}.${format}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error) {
      alert('Export failed');
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="relative">
      <button
        disabled={exporting}
        onClick={() => setShowMenu(!showMenu)}
        className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
      >
        {exporting ? 'Exporting...' : 'Export'}
      </button>
      {showMenu && (
        <div className="absolute right-0 mt-2 w-48 bg-white rounded-md shadow-lg z-10 border border-gray-200">
          <div className="py-1">
            <button
              onClick={() => handleExport('csv')}
              className="block w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
            >
              Export as CSV
            </button>
            <button
              onClick={() => handleExport('pdf')}
              className="block w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
            >
              Export as PDF
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
