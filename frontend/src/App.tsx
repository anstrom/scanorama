import React, { useState } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Activity, Target, Users, Network, Server, AlertCircle, Loader2, Plus, RefreshCw } from 'lucide-react';
import { useHealth, useProfiles, useScans, useHosts, useDiscoveryJobs } from './hooks/useApi';
import WebSocketStatus from './components/WebSocketStatus';
import { useWebSocket } from './hooks/useWebSocket';

// Create QueryClient
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 minutes
      retry: 2,
    },
  },
});

// Enhanced Layout Component with WebSocket status
function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation();

  const navItems = [
    { path: '/', label: 'Dashboard', icon: Activity },
    { path: '/profiles', label: 'Profiles', icon: Target },
    { path: '/scans', label: 'Scans', icon: Server },
    { path: '/hosts', label: 'Hosts', icon: Users },
    { path: '/discovery', label: 'Discovery', icon: Network },
  ];

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900">
      <header className="bg-slate-950/50 backdrop-blur-sm border-b border-slate-800/50 sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between items-center h-16">
            <div className="flex items-center space-x-4">
              <div className="flex items-center space-x-2">
                <div className="w-8 h-8 bg-gradient-to-br from-blue-400 to-blue-600 rounded-lg flex items-center justify-center">
                  <Activity className="w-5 h-5 text-white" />
                </div>
                <h1 className="text-xl font-bold bg-gradient-to-r from-blue-400 to-cyan-400 bg-clip-text text-transparent">
                  Scanorama
                </h1>
              </div>
              <WebSocketStatus showDetails={true} />
            </div>
            <nav className="flex space-x-1">
              {navItems.map(({ path, label, icon: Icon }) => (
                <Link
                  key={path}
                  to={path}
                  className={`inline-flex items-center px-4 py-2 text-sm font-medium rounded-lg transition-all duration-200 ${
                    location.pathname === path
                      ? 'bg-blue-600 text-white shadow-lg shadow-blue-600/20'
                      : 'text-slate-300 hover:text-white hover:bg-slate-700/50'
                  }`}
                >
                  <Icon className="w-4 h-4 mr-2" />
                  {label}
                </Link>
              ))}
            </nav>
          </div>
        </div>
      </header>
      <main className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
        {children}
      </main>
    </div>
  );
}

// Enhanced Dashboard Component with real API data
function Dashboard() {
  const { data: health, isLoading: healthLoading, error: healthError } = useHealth();
  const { data: scans, isLoading: scansLoading } = useScans();
  const { data: hosts, isLoading: hostsLoading } = useHosts();
  const { data: discoveryJobs, isLoading: jobsLoading } = useDiscoveryJobs();

  const getStatusColor = (status?: string) => {
    switch (status) {
      case 'healthy': return 'text-green-400';
      case 'degraded': return 'text-yellow-400';
      case 'unhealthy': return 'text-red-400';
      default: return 'text-gray-400';
    }
  };

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold text-white">Dashboard</h2>
        <div className="text-sm text-slate-400">
          Last updated: {new Date().toLocaleTimeString()}
        </div>
      </div>

      {/* System Status Alert */}
      {healthError && (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-4 flex items-center space-x-3">
          <AlertCircle className="w-5 h-5 text-red-400 flex-shrink-0" />
          <div>
            <h3 className="text-red-400 font-medium">System Health Check Failed</h3>
            <p className="text-red-300 text-sm">Unable to connect to the API. Please check your connection.</p>
          </div>
        </div>
      )}

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        {/* System Status Card */}
        <div className="bg-slate-800/50 backdrop-blur-sm border border-slate-700/50 rounded-xl p-6 card-hover">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-white">System Status</h3>
            <Activity className="w-5 h-5 text-blue-400" />
          </div>
          {healthLoading ? (
            <div className="flex items-center space-x-2">
              <Loader2 className="w-4 h-4 animate-spin text-slate-400" />
              <span className="text-slate-400">Checking...</span>
            </div>
          ) : (
            <div className="space-y-2">
              <p className={`text-xl font-bold ${getStatusColor(health?.status)}`}>
                {health?.status ? health.status.charAt(0).toUpperCase() + health.status.slice(1) : 'Unknown'}
              </p>
              <p className="text-xs text-slate-400">
                Database: {health?.checks?.database || 'unknown'}
              </p>
            </div>
          )}
        </div>

        {/* Active Scans Card */}
        <div className="bg-slate-800/50 backdrop-blur-sm border border-slate-700/50 rounded-xl p-6 card-hover">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-white">Active Scans</h3>
            <Server className="w-5 h-5 text-blue-400" />
          </div>
          {scansLoading ? (
            <div className="flex items-center space-x-2">
              <Loader2 className="w-4 h-4 animate-spin text-slate-400" />
              <span className="text-slate-400">Loading...</span>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-3xl font-bold text-white">
                {scans?.data?.length || 0}
              </p>
              <p className="text-xs text-slate-400">
                Total: {scans?.pagination?.total_items || 0}
              </p>
            </div>
          )}
        </div>

        {/* Total Hosts Card */}
        <div className="bg-slate-800/50 backdrop-blur-sm border border-slate-700/50 rounded-xl p-6 card-hover">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-white">Discovered Hosts</h3>
            <Users className="w-5 h-5 text-blue-400" />
          </div>
          {hostsLoading ? (
            <div className="flex items-center space-x-2">
              <Loader2 className="w-4 h-4 animate-spin text-slate-400" />
              <span className="text-slate-400">Loading...</span>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-3xl font-bold text-white">
                {hosts?.data?.length || 0}
              </p>
              <p className="text-xs text-slate-400">
                Total: {hosts?.pagination?.total_items || 0}
              </p>
            </div>
          )}
        </div>

        {/* Discovery Jobs Card */}
        <div className="bg-slate-800/50 backdrop-blur-sm border border-slate-700/50 rounded-xl p-6 card-hover">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-white">Discovery Jobs</h3>
            <Network className="w-5 h-5 text-blue-400" />
          </div>
          {jobsLoading ? (
            <div className="flex items-center space-x-2">
              <Loader2 className="w-4 h-4 animate-spin text-slate-400" />
              <span className="text-slate-400">Loading...</span>
            </div>
          ) : (
            <div className="space-y-2">
              <p className="text-3xl font-bold text-white">
                {discoveryJobs?.data?.length || 0}
              </p>
              <p className="text-xs text-slate-400">
                Total: {discoveryJobs?.pagination?.total_items || 0}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// Enhanced Profiles Component with real API data
function Profiles() {
  const { data: profiles, isLoading, error, refetch } = useProfiles();
  const [expandedPorts, setExpandedPorts] = useState<Set<string>>(new Set());

  const togglePortsExpansion = (profileId: string) => {
    setExpandedPorts(prev => {
      const newSet = new Set(prev);
      if (newSet.has(profileId)) {
        newSet.delete(profileId);
      } else {
        newSet.add(profileId);
      }
      return newSet;
    });
  };

  if (error) {
    return (
      <div className="space-y-6">
        <h2 className="text-3xl font-bold text-white">Scan Profiles</h2>
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-6 text-center">
          <AlertCircle className="w-12 h-12 text-red-400 mx-auto mb-4" />
          <h3 className="text-red-400 font-medium mb-2">Failed to Load Profiles</h3>
          <p className="text-red-300 text-sm mb-4">There was an error loading scan profiles.</p>
          <button
            onClick={() => refetch()}
            className="btn btn-primary"
          >
            <RefreshCw className="w-4 h-4 mr-2" />
            Try Again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold text-white">Scan Profiles</h2>
        <button className="btn btn-primary">
          <Plus className="w-4 h-4 mr-2" />
          Create Profile
        </button>
      </div>

      {isLoading ? (
        <div className="bg-slate-800/50 rounded-xl p-12 text-center">
          <Loader2 className="w-8 h-8 animate-spin text-blue-400 mx-auto mb-4" />
          <p className="text-slate-400">Loading profiles...</p>
        </div>
      ) : profiles?.data && profiles.data.length > 0 ? (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {profiles.data.map((profile) => (
            <div key={profile.id} className="bg-slate-800/50 border border-slate-700/50 rounded-xl p-6 card-hover">
              <div className="flex items-start justify-between mb-4">
                <h3 className="text-lg font-semibold text-white">{profile.name}</h3>
                {profile.default && (
                  <span className="badge badge-info">Default</span>
                )}
              </div>
              <p className="text-slate-400 text-sm mb-4">{profile.description}</p>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-slate-500">Scan Type:</span>
                  <span className="text-white">{profile.scan_type}</span>
                </div>
                <div className="space-y-2">
                  <span className="text-slate-500 text-sm">Ports:</span>
                  <div className="flex flex-wrap gap-1 max-w-full">
                    {(() => {
                      const ports = profile.ports.split(',');
                      const isExpanded = expandedPorts.has(profile.id);
                      const displayPorts = isExpanded ? ports : ports.slice(0, 8);

                      return (
                        <>
                          {displayPorts.map((port, index) => (
                            <span key={index} className="inline-block px-2 py-1 bg-blue-900/30 text-blue-300 text-xs rounded whitespace-nowrap">
                              {port.trim()}
                            </span>
                          ))}
                          {ports.length > 8 && (
                            <button
                              onClick={() => togglePortsExpansion(profile.id)}
                              className="inline-block px-2 py-1 bg-slate-700 hover:bg-slate-600 text-slate-300 hover:text-white text-xs rounded whitespace-nowrap transition-colors cursor-pointer"
                            >
                              {isExpanded ? 'Show less' : `+${ports.length - 8} more`}
                            </button>
                          )}
                        </>
                      );
                    })()}
                  </div>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Usage:</span>
                  <span className="text-white">{profile.usage_count} times</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="bg-slate-800/50 rounded-xl p-12 text-center">
          <Target className="w-16 h-16 text-slate-600 mx-auto mb-4" />
          <h3 className="text-xl font-medium text-white mb-2">No Scan Profiles</h3>
          <p className="text-slate-400 mb-6">Create your first scan profile to get started with automated scanning.</p>
          <button className="btn btn-primary">
            <Plus className="w-4 h-4 mr-2" />
            Create Profile
          </button>
        </div>
      )}
    </div>
  );
}

// Enhanced Scans Component with real API data
function Scans() {
  const { data: scans, isLoading, error } = useScans();

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'completed': return 'badge badge-success';
      case 'running': return 'badge badge-info';
      case 'failed': return 'badge badge-danger';
      case 'cancelled': return 'badge badge-warning';
      default: return 'badge badge-secondary';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold text-white">Scans</h2>
        <button className="btn btn-primary">
          <Plus className="w-4 h-4 mr-2" />
          New Scan
        </button>
      </div>

      {error ? (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-6 text-center">
          <AlertCircle className="w-12 h-12 text-red-400 mx-auto mb-4" />
          <p className="text-red-400">Failed to load scans</p>
        </div>
      ) : isLoading ? (
        <div className="bg-slate-800/50 rounded-xl p-8 text-center">
          <Loader2 className="w-8 h-8 animate-spin text-blue-400 mx-auto mb-4" />
          <p className="text-slate-400">Loading scans...</p>
        </div>
      ) : scans?.data && scans.data.length > 0 ? (
        <div className="bg-slate-800/50 rounded-xl border border-slate-700/50 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-slate-900/50 border-b border-slate-700">
                <tr>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Name</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Status</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Progress</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Targets</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Started</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700">
                {scans.data.map((scan) => (
                  <tr key={scan.id} className="hover:bg-slate-700/50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-white">{scan.name || `Scan ${scan.id}`}</div>
                      {scan.description && (
                        <div className="text-xs text-slate-400">{scan.description}</div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={getStatusBadge(scan.status)}>{scan.status}</span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center space-x-3">
                        <div className="w-16 bg-slate-700 rounded-full h-2">
                          <div
                            className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                            style={{ width: `${scan.progress}%` }}
                          />
                        </div>
                        <span className="text-sm text-white">{scan.progress}%</span>
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                      {scan.targets.length} target{scan.targets.length !== 1 ? 's' : ''}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-400">
                      {scan.started_at ? new Date(scan.started_at).toLocaleString() : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="bg-slate-800/50 rounded-xl p-12 text-center">
          <Server className="w-16 h-16 text-slate-600 mx-auto mb-4" />
          <h3 className="text-xl font-medium text-white mb-2">No Scans</h3>
          <p className="text-slate-400 mb-6">Start your first network scan to discover vulnerabilities and open ports.</p>
          <button className="btn btn-primary">
            <Plus className="w-4 h-4 mr-2" />
            Create Scan
          </button>
        </div>
      )}
    </div>
  );
}

// Enhanced Hosts Component with real API data
function Hosts() {
  const { data: hosts, isLoading, error } = useHosts();

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'up': return 'text-green-400';
      case 'down': return 'text-red-400';
      case 'scanning': return 'text-blue-400';
      default: return 'text-gray-400';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold text-white">Discovered Hosts</h2>
        <button className="btn btn-secondary">
          <RefreshCw className="w-4 h-4 mr-2" />
          Refresh
        </button>
      </div>

      {error ? (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-6 text-center">
          <AlertCircle className="w-12 h-12 text-red-400 mx-auto mb-4" />
          <p className="text-red-400">Failed to load hosts</p>
        </div>
      ) : isLoading ? (
        <div className="bg-slate-800/50 rounded-xl p-8 text-center">
          <Loader2 className="w-8 h-8 animate-spin text-blue-400 mx-auto mb-4" />
          <p className="text-slate-400">Loading hosts...</p>
        </div>
      ) : hosts?.data && hosts.data.length > 0 ? (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {hosts.data.map((host) => (
            <div key={host.id} className="bg-slate-800/50 border border-slate-700/50 rounded-xl p-6 card-hover">
              <div className="flex items-start justify-between mb-4">
                <div>
                  <h3 className="text-lg font-semibold text-white">{host.ip_address}</h3>
                  {host.hostname && (
                    <p className="text-sm text-slate-400">{host.hostname}</p>
                  )}
                </div>
                <span className={`text-sm font-medium ${getStatusColor(host.status)}`}>
                  {host.status}
                </span>
              </div>

              <div className="space-y-3">
                {host.mac_address && (
                  <div className="flex justify-between text-sm">
                    <span className="text-slate-500">MAC Address:</span>
                    <span className="text-white font-mono">{host.mac_address}</span>
                  </div>
                )}

                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">Open Ports:</span>
                  <span className="text-white">{host.open_ports.length}</span>
                </div>

                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">Scan Count:</span>
                  <span className="text-white">{host.scan_count}</span>
                </div>

                <div className="flex justify-between text-sm">
                  <span className="text-slate-500">Last Seen:</span>
                  <span className="text-slate-400">
                    {new Date(host.last_seen).toLocaleDateString()}
                  </span>
                </div>
              </div>

              {host.open_ports.length > 0 && (
                <div className="mt-4 pt-4 border-t border-slate-700 overflow-hidden">
                  <p className="text-xs text-slate-500 mb-2">Open Ports:</p>
                  <div className="flex flex-wrap gap-1 max-w-full overflow-hidden">
                    {host.open_ports.slice(0, 6).map((port) => (
                      <span key={port} className="inline-block px-2 py-1 bg-blue-900/30 text-blue-300 text-xs rounded whitespace-nowrap">
                        {port}
                      </span>
                    ))}
                    {host.open_ports.length > 6 && (
                      <span className="inline-block px-2 py-1 bg-slate-700 text-slate-300 text-xs rounded whitespace-nowrap">
                        +{host.open_ports.length - 6}
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="bg-slate-800/50 rounded-xl p-12 text-center">
          <Users className="w-16 h-16 text-slate-600 mx-auto mb-4" />
          <h3 className="text-xl font-medium text-white mb-2">No Hosts Discovered</h3>
          <p className="text-slate-400 mb-6">Run a network scan to discover hosts and their open ports.</p>
          <button className="btn btn-primary">
            <Plus className="w-4 h-4 mr-2" />
            Start Discovery
          </button>
        </div>
      )}
    </div>
  );
}

// Enhanced Discovery Component with real API data
function Discovery() {
  const { data: jobs, isLoading, error } = useDiscoveryJobs();

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'active': return 'badge badge-success';
      case 'running': return 'badge badge-info';
      case 'error': return 'badge badge-danger';
      case 'inactive': return 'badge badge-secondary';
      default: return 'badge badge-secondary';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold text-white">Discovery Jobs</h2>
        <button className="btn btn-primary">
          <Plus className="w-4 h-4 mr-2" />
          New Job
        </button>
      </div>

      {error ? (
        <div className="bg-red-900/20 border border-red-800 rounded-lg p-6 text-center">
          <AlertCircle className="w-12 h-12 text-red-400 mx-auto mb-4" />
          <p className="text-red-400">Failed to load discovery jobs</p>
        </div>
      ) : isLoading ? (
        <div className="bg-slate-800/50 rounded-xl p-8 text-center">
          <Loader2 className="w-8 h-8 animate-spin text-blue-400 mx-auto mb-4" />
          <p className="text-slate-400">Loading discovery jobs...</p>
        </div>
      ) : jobs?.data && jobs.data.length > 0 ? (
        <div className="bg-slate-800/50 rounded-xl border border-slate-700/50 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-slate-900/50 border-b border-slate-700">
                <tr>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Name</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Status</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Networks</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Method</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Hosts Found</th>
                  <th className="px-6 py-4 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">Progress</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700">
                {jobs.data.map((job) => (
                  <tr key={job.id} className="hover:bg-slate-700/50 transition-colors">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-white">{job.name}</div>
                      {job.description && (
                        <div className="text-xs text-slate-400">{job.description}</div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className={getStatusBadge(job.status)}>{job.status}</span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                      {job.networks.length} network{job.networks.length !== 1 ? 's' : ''}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-300">
                      {job.method}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-white">
                      {job.hosts_found}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center space-x-3">
                        <div className="w-16 bg-slate-700 rounded-full h-2">
                          <div
                            className="bg-blue-600 h-2 rounded-full transition-all duration-300"
                            style={{ width: `${job.progress}%` }}
                          />
                        </div>
                        <span className="text-sm text-white">{job.progress}%</span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : (
        <div className="bg-slate-800/50 rounded-xl p-12 text-center">
          <Network className="w-16 h-16 text-slate-600 mx-auto mb-4" />
          <h3 className="text-xl font-medium text-white mb-2">No Discovery Jobs</h3>
          <p className="text-slate-400 mb-6">Create your first discovery job to automate network scanning and host discovery.</p>
          <button className="btn btn-primary">
            <Plus className="w-4 h-4 mr-2" />
            Create Discovery Job
          </button>
        </div>
      )}
    </div>
  );
}

// Main App Component
function App() {
  // Initialize WebSocket connection with auto-connect (used for side effects only)
  void useWebSocket({
    autoConnect: true,
    reconnectOnMount: true,
    messageTypes: ['*'] // Listen to all message types
  });

  return (
    <QueryClientProvider client={queryClient}>
      <Router>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/profiles" element={<Profiles />} />
            <Route path="/scans" element={<Scans />} />
            <Route path="/hosts" element={<Hosts />} />
            <Route path="/discovery" element={<Discovery />} />
          </Routes>
        </Layout>
      </Router>
    </QueryClientProvider>
  );
}

export default App;
