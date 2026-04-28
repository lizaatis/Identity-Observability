import { useState } from 'react';
import { remediationAPI } from '../api/client';
import { ExclamationTriangleIcon } from '@heroicons/react/24/outline';

interface RemediationActionProps {
  identityId: number;
  riskFlagId?: number;
  riskType?: string;
}

export default function RemediationAction({ identityId, riskFlagId, riskType }: RemediationActionProps) {
  const [showDropdown, setShowDropdown] = useState(false);
  const [loading, setLoading] = useState(false);
  const [, setActionType] = useState('');
  const [, setTargetSystem] = useState('');
  const [requiresApproval, setRequiresApproval] = useState(true);

  const handleRemediate = async (type: string, system: string) => {
    setLoading(true);
    setActionType(type);
    setTargetSystem(system);

    try {
      await remediationAPI.createAction(identityId, {
        action_type: type,
        target_system: system,
        requires_approval: requiresApproval,
        metadata: {
          risk_flag_id: riskFlagId,
          risk_type: riskType,
        },
      });
      alert(`Remediation action "${type}" created successfully${requiresApproval ? ' (pending approval)' : ''}`);
      setShowDropdown(false);
    } catch (error: any) {
      alert(`Failed to create remediation action: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative inline-block">
      <button
        onClick={() => setShowDropdown(!showDropdown)}
        disabled={loading}
        className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50 flex items-center space-x-2"
      >
        <ExclamationTriangleIcon className="h-5 w-5" />
        <span>Remediate</span>
      </button>

      {showDropdown && (
        <div className="absolute right-0 mt-2 w-64 bg-white rounded-lg shadow-lg border border-gray-200 z-10">
          <div className="p-4 space-y-2">
            <div className="text-sm font-medium text-gray-700 mb-3">Remediation Options</div>

            {/* Approval Toggle */}
            <label className="flex items-center space-x-2 text-sm text-black">
              <input
                type="checkbox"
                checked={requiresApproval}
                onChange={(e) => setRequiresApproval(e.target.checked)}
                className="rounded"
              />
              <span>Require Approval</span>
            </label>

            <div className="border-t pt-2 space-y-1">
              <button
                onClick={() => handleRemediate('disable_user', 'okta')}
                className="w-full text-left px-3 py-2 text-sm text-black hover:bg-gray-100 rounded"
              >
                🚫 Disable User (Okta)
              </button>
              <button
                onClick={() => handleRemediate('disable_user', 'sailpoint')}
                className="w-full text-left px-3 py-2 text-sm text-black hover:bg-gray-100 rounded"
              >
                🚫 Disable User (SailPoint)
              </button>
              <button
                onClick={() => handleRemediate('remove_permission', 'okta')}
                className="w-full text-left px-3 py-2 text-sm text-black hover:bg-gray-100 rounded"
              >
                🔒 Remove Permission
              </button>
              <button
                onClick={() => handleRemediate('remove_group', 'okta')}
                className="w-full text-left px-3 py-2 text-sm text-black hover:bg-gray-100 rounded"
              >
                👥 Remove from Group
              </button>
              <button
                onClick={() => handleRemediate('create_ticket', 'jira')}
                className="w-full text-left px-3 py-2 text-sm text-black hover:bg-gray-100 rounded"
              >
                📋 Create Jira Ticket
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
