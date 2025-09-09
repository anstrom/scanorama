import React from 'react';
import { Network, RefreshCw, Plus, AlertCircle, Loader2 } from 'lucide-react';
import { useDiscoveryJobs } from '../hooks/useApi';

const Jobs: React.FC = () => {
  const { data: jobsData, isLoading, error, refetch } = useDiscoveryJobs();

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active':
        return 'bg-green-900/20 border border-green-800 text-green-400';
      case 'running':
        return 'bg-blue-900/20 border border-blue-800 text-blue-400';
      case 'error':
        return 'bg-red-900/20 border border-red-800 text-red-400';
      case 'inactive':
        return 'bg-gray-900/20 border border-gray-800 text-gray-400';
      default:
        return 'bg-gray-900/20 border border-gray-800 text-gray-400';
    }
  };

  if (error) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900">
        <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
          <div className="text-center">
            <h1 className="text-3xl font-bold text-white mb-4">Discovery Jobs</h1>
            <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-6">
              <AlertCircle className="w-12 h-12 text-red-400 mx-auto mb-4" />
              <p className="text-red-400 mb-4">Failed to load discovery jobs</p>
              <button
                onClick={() => refetch()}
                className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 transition-colors"
              >
                Try Again
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900">
      <div className="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center mb-4">
            <Network className="w-8 h-8 mr-3 text-blue-500" />
            <div>
              <h1 className="text-3xl font-bold text-white">Discovery Jobs</h1>
              <p className="text-lg font-normal text-slate-400 mt-1">
                Manage automated network discovery and host scanning jobs
              </p>
            </div>
          </div>

          <div className="flex justify-between items-center">
            <button
              onClick={() => refetch()}
              className="flex items-center px-4 py-2 bg-slate-700 text-white rounded-lg hover:bg-slate-600 transition-colors"
            >
              <RefreshCw className="w-4 h-4 mr-2" />
              Refresh
            </button>

            <button className="flex items-center px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors">
              <Plus className="w-4 h-4 mr-2" />
              New Discovery Job
            </button>
          </div>
        </div>

        {/* Main Content */}
        <div className="bg-slate-800/50 backdrop-blur-sm border border-slate-700/50 rounded-xl overflow-hidden">
          {isLoading ? (
            <div className="p-8 text-center">
              <Loader2 className="w-12 h-12 animate-spin text-blue-500 mx-auto mb-4" />
              <p className="text-slate-400">Loading discovery jobs...</p>
            </div>
          ) : jobsData?.data && jobsData.data.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="bg-slate-950 border-b border-slate-800">
                  <tr>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Job
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Status
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Networks
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Method
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Progress
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Hosts Found
                    </th>
                    <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                      Last Update
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800">
                  {jobsData.data.map((job) => (
                    <tr key={job.id} className="hover:bg-white/5 transition-colors">
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm font-medium text-white">{job.name}</div>
                        {job.description && (
                          <div className="text-xs text-slate-400">{job.description}</div>
                        )}
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <span className={`px-2 py-1 text-xs rounded-full ${getStatusBadge(job.status)}`}>
                          {job.status}
                        </span>
                      </td>
                      <td className="px-6 py-4">
                        <div className="text-sm text-slate-300">
                          {job.networks.join(', ')}
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm text-slate-300">{job.method}</div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="flex items-center">
                          <div className="w-16 bg-slate-700 rounded-full h-2 mr-2">
                            <div
                              className="bg-blue-500 h-2 rounded-full transition-all duration-300"
                              style={{ width: `${job.progress}%` }}
                            />
                          </div>
                          <span className="text-xs text-slate-400">{job.progress}%</span>
                        </div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm text-slate-300">{job.hosts_found}</div>
                      </td>
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm text-slate-400">
                          {new Date(job.updated_at).toLocaleDateString()}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="p-12 text-center">
              <Network className="w-16 h-16 text-slate-600 mx-auto mb-4" />
              <h3 className="text-xl font-medium text-white mb-2">No Discovery Jobs</h3>
              <p className="text-slate-400 mb-6">
                Create your first discovery job to automate network scanning and host discovery.
              </p>
              <button className="flex items-center mx-auto px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors">
                <Plus className="w-5 h-5 mr-2" />
                Create Discovery Job
              </button>
            </div>
          )}

          {/* Pagination Info */}
          {jobsData?.pagination && jobsData.pagination.total_items > 0 && (
            <div className="px-6 py-4 border-t border-slate-800 bg-slate-950/50">
              <div className="text-sm text-slate-400">
                Showing {jobsData.data.length} of {jobsData.pagination.total_items} discovery jobs
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default Jobs;
