import React, { useState, useEffect, useCallback } from 'react'
import { Search, ExternalLink, Code, AlertCircle, Loader2, ChevronRight, Wrench, Eye, CheckCircle, Clock, Settings } from 'lucide-react'
import { mcpRegistryApi, type MCPRegistryServer, type EnhancedMCPRegistryServer, type RegistryServerTools } from '../services/mcpRegistryApi'

interface MCPRegistryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenConfigEditor: () => void;
}

export default function MCPRegistryModal({ isOpen, onClose, onOpenConfigEditor }: MCPRegistryModalProps) {
  const [servers, setServers] = useState<EnhancedMCPRegistryServer[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedServer, setSelectedServer] = useState<EnhancedMCPRegistryServer | null>(null)
  const [serverTools, setServerTools] = useState<RegistryServerTools | null>(null)
  const [loadingTools, setLoadingTools] = useState<Set<string>>(new Set())
  const [toolsError, setToolsError] = useState<string | null>(null)
  
  // Authentication input state
  const [customHeaders, setCustomHeaders] = useState<Record<string, string>>({})
  const [customEnvVars, setCustomEnvVars] = useState<Record<string, string>>({})
  const [showAuthSection, setShowAuthSection] = useState(false)
  
  // Pagination state
  const [pageSize, setPageSize] = useState(100)
  const [totalCount, setTotalCount] = useState(0)
  const [hasNextPage, setHasNextPage] = useState(false)
  const [nextCursor, setNextCursor] = useState<string | null>(null)

  // Keyboard shortcuts
  useEffect(() => {
    if (!isOpen) return

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, onClose])


  // Search servers with cursor-based pagination
  const searchServers = useCallback(async (cursor: string | null = null, append: boolean = false) => {
    // searchServers called
    setLoading(true)
    setError(null)
    try {
      // Making API call
      const response = await mcpRegistryApi.searchServers({
        query: searchQuery || undefined,
        limit: pageSize,
        cursor: cursor || undefined
      })
      // API response
      
      if (append && cursor) {
        // For "Load More" functionality, append new servers to existing ones
        // Appending servers
        setServers(prevServers => {
          const newServers = [...prevServers, ...(response.servers || [])]
          // Total servers after append
          return newServers
        })
      } else {
        // For initial load or new search, replace servers
        // Replacing servers
        setServers(response.servers || [])
      }
      
      setTotalCount(response.metadata?.count || 0)
      const newNextCursor = response.metadata?.next_cursor || null
      // Setting nextCursor
      
      // Check if we've reached the end (same cursor means no more pages)
      if (append && cursor && newNextCursor === cursor) {
        // Reached end of results
        setNextCursor(null)
        setHasNextPage(false)
      } else {
        setNextCursor(newNextCursor)
        setHasNextPage(!!newNextCursor)
      }
    } catch (error) {
      console.error('Failed to search servers:', error)
      setError('Failed to load servers from registry. Please try again.')
    } finally {
      setLoading(false)
    }
  }, [searchQuery, pageSize])

  // Load servers on mount
  useEffect(() => {
    if (isOpen) {
      searchServers(null, false) // Load initial servers
    }
  }, [isOpen, searchServers])

  // Handle search form submission
  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    searchServers(null, false)
  }

  // Handle page size change
  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize)
    searchServers(null, false)
  }


  // Install functionality removed

  // Show server details and auto-load tools
  const showServerDetails = (server: MCPRegistryServer) => {
    setSelectedServer(server)
    setServerTools(null) // Reset tools when switching servers
    setToolsError(null)
    
    // Check if authentication is required before auto-loading tools
    const requiredHeaders = getRequiredHeaders(server)
    const requiredEnvVars = getRequiredEnvVars(server)
    const hasAuth = requiredHeaders.length > 0 || requiredEnvVars.length > 0
    
    if (hasAuth) {
      // Show authentication section but don't auto-load tools
      setShowAuthSection(true)
    } else {
      // No authentication required, auto-load tools
      loadServerTools(server)
    }
  }

  // Helper functions for per-server loading state
  const isServerLoading = (serverId: string) => loadingTools.has(serverId)
  
  const setServerLoading = (serverId: string, loading: boolean) => {
    setLoadingTools(prev => {
      const newSet = new Set(prev)
      if (loading) {
        newSet.add(serverId)
      } else {
        newSet.delete(serverId)
      }
      return newSet
    })
  }


  // Load server tools
  // Extract required headers from server configuration
  const getRequiredHeaders = (server: MCPRegistryServer) => {
    const headers: Array<{name: string, description: string, isRequired: boolean, isSecret: boolean}> = []
    
    // Check remotes for headers
    if (server.remotes) {
      server.remotes.forEach(remote => {
        if (remote.headers) {
          remote.headers.forEach(header => {
            headers.push({
              name: header.name,
              description: header.description || '',
              isRequired: header.isRequired || false,
              isSecret: header.isSecret || false
            })
          })
        }
      })
    }
    
    return headers
  }

  // Extract required environment variables from server configuration
  const getRequiredEnvVars = (server: MCPRegistryServer) => {
    const envVars: Array<{name: string, description: string, isRequired: boolean, isSecret: boolean}> = []
    
    // Check packages for environment variables
    if (server.packages) {
      server.packages.forEach(pkg => {
        if (pkg.environmentVariables) {
          pkg.environmentVariables.forEach(envVar => {
            envVars.push({
              name: envVar.name,
              description: envVar.description || '',
              isRequired: envVar.isRequired || false,
              isSecret: envVar.isSecret || false
            })
          })
        }
      })
    }
    
    return envVars
  }

  // Update header value
  const updateHeader = (headerName: string, value: string) => {
    setCustomHeaders(prev => ({
      ...prev,
      [headerName]: value
    }))
  }

  // Update environment variable value
  const updateEnvVar = (envVarName: string, value: string) => {
    setCustomEnvVars(prev => ({
      ...prev,
      [envVarName]: value
    }))
  }

  const loadServerTools = async (server: MCPRegistryServer) => {
    // Use server UUID instead of server name
    const serverId = server._meta?.["io.modelcontextprotocol.registry/official"]?.serverId
    if (!serverId) {
      const errorMsg = 'Server ID not found in registry data'
      setToolsError(errorMsg)
      return
    }

    setServerLoading(serverId, true)
    setToolsError(null)
    try {
      const tools = await mcpRegistryApi.getServerTools(serverId, {
        headers: customHeaders,
        envVars: customEnvVars
      })
      setServerTools(tools)
    } catch (error) {
      console.error('Failed to load server tools:', error)
      const errorMessage = error instanceof Error ? error.message : 'Unknown error'
      
      // Show raw error message directly
      setToolsError(errorMessage)
    } finally {
      setServerLoading(serverId, false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 shadow-xl w-full max-w-6xl h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-xl font-semibold text-gray-900 dark:text-gray-100">
              MCP Server Registry
            </h3>
            <p className="text-sm text-gray-600 dark:text-gray-400 mt-1">
              Discover MCP servers from the official registry
            </p>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={onOpenConfigEditor}
              className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 rounded-md transition-colors flex items-center gap-2 text-sm"
            >
              <Settings className="w-4 h-4" />
              Configure Servers
            </button>
            <button 
              onClick={onClose}
              className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 text-2xl"
            >
              ✕
            </button>
          </div>
        </div>

        {/* Search and Filters */}
        <form onSubmit={handleSearch} className="flex gap-4 mb-6">
          <div className="flex-1 relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              placeholder="Search servers by name, description, or tags..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-sm text-gray-600 dark:text-gray-400">Per page:</label>
            <select
              value={pageSize}
              onChange={(e) => handlePageSizeChange(Number(e.target.value))}
              disabled={loading}
              className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 text-sm"
            >
              <option value={10}>10</option>
              <option value={25}>25</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </div>
          <button
            type="submit"
            disabled={loading}
            className="px-6 py-2 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {loading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Searching...
              </>
            ) : (
              <>
                <Search className="w-4 h-4" />
                Search
              </>
            )}
          </button>
        </form>

        {/* Error Message */}
        {error && (
          <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
            <div className="flex items-center gap-2">
              <AlertCircle className="w-4 h-4 text-red-500" />
              <span className="text-sm text-red-700 dark:text-red-400">{error}</span>
            </div>
          </div>
        )}

        {/* Tool Discovery Warning */}
        {loadingTools.size > 0 && (
          <div className="mb-4 p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md">
            <div className="flex items-center gap-2">
              <AlertCircle className="w-4 h-4 text-yellow-500" />
              <div>
                <span className="text-sm font-medium text-yellow-700 dark:text-yellow-400">
                  Discovering tools...
                </span>
                <p className="text-xs text-yellow-600 dark:text-yellow-500 mt-1">
                  This may take 10-30 seconds as we connect to the server and discover available tools.
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Server List */}
        <div className="space-y-4">
          {servers.length === 0 && !loading ? (
            <div className="text-center py-8">
              <Search className="w-12 h-12 text-gray-400 mx-auto mb-4" />
              <p className="text-gray-500 dark:text-gray-400">
                No servers found. Try adjusting your search criteria.
              </p>
            </div>
          ) : (
            servers.map((server, index) => (
              <div key={server.name || index} className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:shadow-md transition-shadow">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h4 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                        {server.name}
                      </h4>
                      <span className="text-xs bg-gray-100 dark:bg-gray-700 px-2 py-1 rounded">
                        v{server.version}
                      </span>
                      {server.cacheStatus?.isCached && (
                        <div className="flex items-center gap-1 px-2 py-1 bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 rounded text-xs">
                          <CheckCircle className="w-3 h-3" />
                          <span>Cached</span>
                        </div>
                      )}
                      {server.status && (
                        <span className={`text-xs px-2 py-1 rounded ${
                          server.status === 'active' 
                            ? 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300'
                            : 'bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300'
                        }`}>
                          {server.status}
                        </span>
                      )}
                    </div>
                    
                    <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
                      {server.description}
                    </p>
                    
                    {/* Cache Status Information */}
                    {server.cacheStatus?.isCached && (
                      <div className="mb-3 p-2 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-md">
                        <div className="flex items-center gap-2 mb-1">
                          <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400" />
                          <span className="text-sm font-medium text-green-800 dark:text-green-200">
                            Already Installed
                          </span>
                        </div>
                        <div className="text-xs text-green-700 dark:text-green-300 space-y-1">
                          <div className="flex items-center gap-4">
                            <span>{server.cacheStatus.toolsCount || 0} tools</span>
                            <span>{server.cacheStatus.promptsCount || 0} prompts</span>
                            <span>{server.cacheStatus.resourcesCount || 0} resources</span>
                          </div>
                          {server.cacheStatus.lastUpdated && (
                            <div className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              <span>Last updated: {new Date(server.cacheStatus.lastUpdated).toLocaleString()}</span>
                            </div>
                          )}
                        </div>
                      </div>
                    )}
                    
                    <div className="flex items-center gap-2 mb-3 flex-wrap">
                      {server.repository && (
                        <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 px-2 py-1 rounded">
                          {server.repository.source}
                        </span>
                      )}
                      {server.packages && server.packages.length > 0 && (
                        <span className="text-xs bg-purple-100 dark:bg-purple-900 text-purple-700 dark:text-purple-300 px-2 py-1 rounded">
                          {server.packages[0].registryType}
                        </span>
                      )}
                      {server.packages && server.packages[0].environmentVariables && server.packages[0].environmentVariables.length > 0 && (
                        <span className="text-xs bg-orange-100 dark:bg-orange-900 text-orange-700 dark:text-orange-300 px-2 py-1 rounded">
                          {server.packages[0].environmentVariables.length} env vars
                        </span>
                      )}
                      {server.packages && server.packages[0].packageArguments && server.packages[0].packageArguments.length > 0 && (
                        <span className="text-xs bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300 px-2 py-1 rounded">
                          {server.packages[0].packageArguments.length} args
                        </span>
                      )}
                    </div>

                    <div className="flex items-center gap-4 text-xs text-gray-500 dark:text-gray-400 flex-wrap">
                      {server.websiteUrl && (
                        <a
                          href={server.websiteUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1 hover:text-blue-600 dark:hover:text-blue-400"
                        >
                          <ExternalLink className="w-3 h-3" />
                          Website
                        </a>
                      )}
                      {server.repository && (
                        <a
                          href={server.repository.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1 hover:text-blue-600 dark:hover:text-blue-400"
                        >
                          <ExternalLink className="w-3 h-3" />
                          Repository
                        </a>
                      )}
                      {server.packages && server.packages.length > 0 && (
                        <span className="flex items-center gap-1">
                          <Code className="w-3 h-3" />
                          {server.packages[0].identifier}
                        </span>
                      )}
                    </div>
                  </div>
                  
                  <div className="flex items-center gap-2 ml-4">
                    <button
                      onClick={() => showServerDetails(server)}
                      disabled={isServerLoading(server._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '')}
                      className="px-3 py-1 text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded hover:bg-blue-200 dark:hover:bg-blue-800 disabled:opacity-50 flex items-center gap-1"
                    >
                      {isServerLoading(server._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '') ? (
                        <Loader2 className="w-3 h-3 animate-spin" />
                      ) : (
                        <Eye className="w-3 h-3" />
                      )}
                      {server.cacheStatus?.isCached ? 'View Cached Tools' : 'Details & Preview Tools'}
                    </button>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>

        {/* Load More Button - Simple and clean */}
        {servers.length > 0 && hasNextPage && (
          <div className="mt-6 flex justify-center">
            <button
              onClick={() => {
                // Load More clicked
                searchServers(nextCursor, true)
              }}
              disabled={loading}
              className="px-6 py-3 bg-blue-500 text-white rounded-md hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 font-medium"
            >
              {loading ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  Loading...
                </>
              ) : (
                <>
                  <ChevronRight className="w-4 h-4" />
                  Load More Servers
                </>
              )}
            </button>
          </div>
        )}

        {/* Results Counter - Simple info at bottom */}
        {servers.length > 0 && (
          <div className="mt-4 text-center text-sm text-gray-600 dark:text-gray-400">
            Showing {servers.length} servers{totalCount > 0 && ` of ${totalCount} total`}
            {hasNextPage && ' • More available'}
          </div>
        )}

        {/* Server Details Modal */}
        {selectedServer && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-60 p-4">
            <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6 shadow-xl w-full max-w-2xl max-h-[80vh] overflow-y-auto">
              <div className="flex items-center justify-between mb-4">
                <h4 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
                  {selectedServer.name} Details
                </h4>
                <button 
                  onClick={() => setSelectedServer(null)}
                  className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
                >
                  ✕
                </button>
              </div>
              
              <div className="space-y-4">
                <div>
                  <h5 className="font-medium text-gray-900 dark:text-gray-100 mb-2">Description</h5>
                  <p className="text-sm text-gray-600 dark:text-gray-400">{selectedServer.description}</p>
                </div>

                {selectedServer.websiteUrl && (
                  <div>
                    <h5 className="font-medium text-gray-900 dark:text-gray-100 mb-2">Website</h5>
                    <a
                      href={selectedServer.websiteUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-sm text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
                    >
                      <ExternalLink className="w-3 h-3" />
                      {selectedServer.websiteUrl}
                    </a>
                  </div>
                )}
                
                {selectedServer.packages && selectedServer.packages.length > 0 && (
                  <div>
                    <h5 className="font-medium text-gray-900 dark:text-gray-100 mb-2">Installation</h5>
                    <div className="space-y-4">
                      {selectedServer.packages.map((pkg, index) => (
                        <div key={index} className="bg-gray-100 dark:bg-gray-700 p-4 rounded-lg">
                          <div className="flex items-center gap-2 mb-3">
                            <span className="text-sm font-medium">{pkg.registryType}</span>
                            <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 px-2 py-1 rounded">
                              {pkg.identifier}
                            </span>
                            <span className="text-xs text-gray-500">v{pkg.version}</span>
                          </div>
                          
                          <div className="mb-3">
                            <code className="text-sm bg-gray-200 dark:bg-gray-600 px-2 py-1 rounded">
                              {pkg.registryType === 'npm' ? `npx ${pkg.identifier}` : 
                               pkg.registryType === 'pypi' ? `pip install ${pkg.identifier}` :
                               pkg.registryType === 'nuget' ? `dotnet add package ${pkg.identifier}` :
                               `${pkg.identifier}`}
                            </code>
                          </div>

                          {pkg.runtimeHint && (
                            <div className="mb-3">
                              <h6 className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Runtime Hint:</h6>
                              <p className="text-xs text-gray-600 dark:text-gray-400">{pkg.runtimeHint}</p>
                            </div>
                          )}

                          {pkg.environmentVariables && pkg.environmentVariables.length > 0 && (
                            <div className="mb-3">
                              <h6 className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-2">Environment Variables ({pkg.environmentVariables.length}):</h6>
                              <div className="space-y-2">
                                {pkg.environmentVariables.map((envVar, envIndex) => (
                                  <div key={envIndex} className="bg-white dark:bg-gray-800 p-2 rounded border">
                                    <div className="flex items-center gap-2 mb-1">
                                      <span className="font-mono text-xs font-medium">{envVar.name}</span>
                                      {envVar.isRequired && (
                                        <span className="text-xs bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 px-1 py-0.5 rounded">
                                          required
                                        </span>
                                      )}
                                      {envVar.isSecret && (
                                        <span className="text-xs bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300 px-1 py-0.5 rounded">
                                          secret
                                        </span>
                                      )}
                                    </div>
                                    <p className="text-xs text-gray-600 dark:text-gray-400 mb-1">{envVar.description}</p>
                                    {envVar.default && (
                                      <p className="text-xs text-gray-500">Default: <code>{envVar.default}</code></p>
                                    )}
                                    {envVar.choices && envVar.choices.length > 0 && (
                                      <p className="text-xs text-gray-500">Choices: {envVar.choices.join(', ')}</p>
                                    )}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          {pkg.packageArguments && pkg.packageArguments.length > 0 && (
                            <div className="mb-3">
                              <h6 className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-2">Package Arguments ({pkg.packageArguments.length}):</h6>
                              <div className="space-y-2">
                                {pkg.packageArguments.map((arg, argIndex) => (
                                  <div key={argIndex} className="bg-white dark:bg-gray-800 p-2 rounded border">
                                    <div className="flex items-center gap-2 mb-1">
                                      <span className="font-mono text-xs font-medium">{arg.name}</span>
                                      <span className="text-xs bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 px-1 py-0.5 rounded">
                                        {arg.type}
                                      </span>
                                      {arg.isRequired && (
                                        <span className="text-xs bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 px-1 py-0.5 rounded">
                                          required
                                        </span>
                                      )}
                                      {arg.isRepeated && (
                                        <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 px-1 py-0.5 rounded">
                                          repeated
                                        </span>
                                      )}
                                      {arg.isSecret && (
                                        <span className="text-xs bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300 px-1 py-0.5 rounded">
                                          secret
                                        </span>
                                      )}
                                    </div>
                                    <p className="text-xs text-gray-600 dark:text-gray-400 mb-1">{arg.description}</p>
                                    {arg.default && (
                                      <p className="text-xs text-gray-500">Default: <code>{arg.default}</code></p>
                                    )}
                                    {arg.valueHint && (
                                      <p className="text-xs text-gray-500">Hint: {arg.valueHint}</p>
                                    )}
                                    {arg.choices && arg.choices.length > 0 && (
                                      <p className="text-xs text-gray-500">Choices: {arg.choices.join(', ')}</p>
                                    )}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}

                          {pkg.runtimeArguments && pkg.runtimeArguments.length > 0 && (
                            <div className="mb-3">
                              <h6 className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-2">Runtime Arguments ({pkg.runtimeArguments.length}):</h6>
                              <div className="space-y-2">
                                {pkg.runtimeArguments.map((arg, argIndex) => (
                                  <div key={argIndex} className="bg-white dark:bg-gray-800 p-2 rounded border">
                                    <div className="flex items-center gap-2 mb-1">
                                      <span className="font-mono text-xs font-medium">{arg.name}</span>
                                      <span className="text-xs bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 px-1 py-0.5 rounded">
                                        {arg.type}
                                      </span>
                                      {arg.isRequired && (
                                        <span className="text-xs bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 px-1 py-0.5 rounded">
                                          required
                                        </span>
                                      )}
                                      {arg.isRepeated && (
                                        <span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 px-1 py-0.5 rounded">
                                          repeated
                                        </span>
                                      )}
                                      {arg.isSecret && (
                                        <span className="text-xs bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300 px-1 py-0.5 rounded">
                                          secret
                                        </span>
                                      )}
                                    </div>
                                    <p className="text-xs text-gray-600 dark:text-gray-400 mb-1">{arg.description}</p>
                                    {arg.default && (
                                      <p className="text-xs text-gray-500">Default: <code>{arg.default}</code></p>
                                    )}
                                    {arg.valueHint && (
                                      <p className="text-xs text-gray-500">Hint: {arg.valueHint}</p>
                                    )}
                                    {arg.choices && arg.choices.length > 0 && (
                                      <p className="text-xs text-gray-500">Choices: {arg.choices.join(', ')}</p>
                                    )}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}
                
                {selectedServer.remotes && selectedServer.remotes.length > 0 && (
                  <div>
                    <h5 className="font-medium text-gray-900 dark:text-gray-100 mb-2">Remote Endpoints</h5>
                    <div className="space-y-2">
                      {selectedServer.remotes.map((remote, index) => (
                        <div key={index} className="bg-gray-100 dark:bg-gray-700 p-3 rounded">
                          <div className="flex items-center gap-2 mb-2">
                            <span className="text-sm font-medium">{remote.type}</span>
                          </div>
                          <code className="text-sm bg-gray-200 dark:bg-gray-600 px-2 py-1 rounded block mb-2">{remote.url}</code>
                          {remote.headers && remote.headers.length > 0 && (
                            <div>
                              <h6 className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-2">Headers ({remote.headers.length}):</h6>
                              <div className="space-y-2">
                                {remote.headers.map((header, headerIndex) => (
                                  <div key={headerIndex} className="bg-white dark:bg-gray-800 p-2 rounded border">
                                    <div className="flex items-center gap-2 mb-1">
                                      <span className="font-mono text-xs font-medium">{header.name}</span>
                                      {header.isRequired && (
                                        <span className="text-xs bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 px-1 py-0.5 rounded">
                                          required
                                        </span>
                                      )}
                                      {header.isSecret && (
                                        <span className="text-xs bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300 px-1 py-0.5 rounded">
                                          secret
                                        </span>
                                      )}
                                    </div>
                                    <p className="text-xs text-gray-600 dark:text-gray-400 mb-1">{header.description}</p>
                                    {header.default && (
                                      <p className="text-xs text-gray-500">Default: <code>{header.default}</code></p>
                                    )}
                                    {header.choices && header.choices.length > 0 && (
                                      <p className="text-xs text-gray-500">Choices: {header.choices.join(', ')}</p>
                                    )}
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Authentication Section */}
                {(() => {
                  const requiredHeaders = getRequiredHeaders(selectedServer)
                  const requiredEnvVars = getRequiredEnvVars(selectedServer)
                  const hasAuth = requiredHeaders.length > 0 || requiredEnvVars.length > 0
                  
                  if (hasAuth) {
                    return (
                      <div>
                        <div className="flex items-center justify-between mb-4">
                          <h5 className="font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
                            <AlertCircle className="w-4 h-4" />
                            Authentication Required
                          </h5>
                          <button
                            onClick={() => setShowAuthSection(!showAuthSection)}
                            className="px-2 py-1 text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded hover:bg-blue-200 dark:hover:bg-blue-800 transition-colors"
                          >
                            {showAuthSection ? 'Hide' : 'Show'} Authentication
                          </button>
                        </div>
                        
                        {showAuthSection && (
                          <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4 mb-4">
                            <div className="flex items-start gap-3">
                              <div className="flex-shrink-0">
                                <AlertCircle className="w-5 h-5 text-blue-500 mt-0.5" />
                              </div>
                              <div className="flex-1">
                                <h6 className="text-sm font-medium text-blue-900 dark:text-blue-100 mb-2">
                                  Authentication Required
                                </h6>
                                <p className="text-sm text-blue-700 dark:text-blue-300 mb-4">
                                  This server requires authentication. Please provide the required credentials below and click "Load Tools with Auth" to preview tools.
                                </p>
                                
                                <div className="space-y-4">
                                  {/* Headers Section */}
                                  {requiredHeaders.length > 0 && (
                                    <div className="bg-white dark:bg-gray-800/50 border border-blue-200 dark:border-blue-700 rounded-md p-3">
                                      <h6 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3 flex items-center gap-2">
                                        <span className="w-2 h-2 bg-blue-500 rounded-full"></span>
                                        Headers ({requiredHeaders.length})
                                      </h6>
                                      <div className="space-y-3">
                                        {requiredHeaders.map((header, index) => (
                                          <div key={index} className="space-y-1">
                                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                                              {header.name}
                                              {header.isRequired && (
                                                <span className="text-red-500 ml-1">*</span>
                                              )}
                                              {header.isSecret && (
                                                <span className="text-yellow-500 ml-1">🔒</span>
                                              )}
                                            </label>
                                            <input
                                              type={header.isSecret ? 'password' : 'text'}
                                              value={customHeaders[header.name] || ''}
                                              onChange={(e) => updateHeader(header.name, e.target.value)}
                                              placeholder={`Enter ${header.name}${header.isRequired ? ' (required)' : ''}`}
                                              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 text-sm"
                                            />
                                            {header.description && (
                                              <p className="text-xs text-gray-500 dark:text-gray-400">{header.description}</p>
                                            )}
                                          </div>
                                        ))}
                                      </div>
                                    </div>
                                  )}
                                  
                                  {/* Environment Variables Section */}
                                  {requiredEnvVars.length > 0 && (
                                    <div className="bg-white dark:bg-gray-800/50 border border-blue-200 dark:border-blue-700 rounded-md p-3">
                                      <h6 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3 flex items-center gap-2">
                                        <span className="w-2 h-2 bg-green-500 rounded-full"></span>
                                        Environment Variables ({requiredEnvVars.length})
                                      </h6>
                                      <div className="space-y-3">
                                        {requiredEnvVars.map((envVar, index) => (
                                          <div key={index} className="space-y-1">
                                            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
                                              {envVar.name}
                                              {envVar.isRequired && (
                                                <span className="text-red-500 ml-1">*</span>
                                              )}
                                              {envVar.isSecret && (
                                                <span className="text-yellow-500 ml-1">🔒</span>
                                              )}
                                            </label>
                                            <input
                                              type={envVar.isSecret ? 'password' : 'text'}
                                              value={customEnvVars[envVar.name] || ''}
                                              onChange={(e) => updateEnvVar(envVar.name, e.target.value)}
                                              placeholder={`Enter ${envVar.name}${envVar.isRequired ? ' (required)' : ''}`}
                                              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 text-sm"
                                            />
                                            {envVar.description && (
                                              <p className="text-xs text-gray-500 dark:text-gray-400">{envVar.description}</p>
                                            )}
                                          </div>
                                        ))}
                                      </div>
                                    </div>
                                  )}
                                </div>
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    )
                  }
                  return null
                })()}

                {/* Tools Preview Section */}
                <div>
                  <div className="flex items-center justify-between mb-4">
                    <h5 className="font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
                      <Wrench className="w-4 h-4" />
                      Available Tools
                    </h5>
                    <button
                      onClick={() => loadServerTools(selectedServer)}
                      disabled={isServerLoading(selectedServer._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '')}
                      className="px-3 py-1 text-xs bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded hover:bg-blue-200 dark:hover:bg-blue-800 disabled:opacity-50 flex items-center gap-1"
                    >
                      {isServerLoading(selectedServer._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '') ? (
                        <Loader2 className="w-3 h-3 animate-spin" />
                      ) : (
                        <Eye className="w-3 h-3" />
                      )}
                      {(() => {
                        const requiredHeaders = getRequiredHeaders(selectedServer)
                        const requiredEnvVars = getRequiredEnvVars(selectedServer)
                        const hasAuth = requiredHeaders.length > 0 || requiredEnvVars.length > 0
                        if (serverTools) {
                          return hasAuth ? 'Refresh Tools with Auth' : 'Refresh Tools'
                        } else {
                          return hasAuth ? 'Load Tools with Auth' : 'Load Tools'
                        }
                      })()}
                    </button>
                  </div>

                  {toolsError && (
                    <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
                      <div className="flex items-center gap-2">
                        <AlertCircle className="w-4 h-4 text-red-500" />
                        <span className="text-sm text-red-700 dark:text-red-400">{toolsError}</span>
                      </div>
                    </div>
                  )}

                  {isServerLoading(selectedServer._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '') && (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="w-6 h-6 animate-spin text-blue-500" />
                      <span className="ml-2 text-sm text-gray-600 dark:text-gray-400">
                        Discovering tools for {selectedServer.name}...
                      </span>
                    </div>
                  )}

                  {serverTools && !isServerLoading(selectedServer._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '') && (
                    <div className="space-y-4">
                      <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
                        <span className="font-medium">{serverTools.toolsEnabled}</span>
                        <span>tools available</span>
                        {serverTools.status === 'ok' && (
                          <span className="text-green-600 dark:text-green-400">• Connected</span>
                        )}
                      </div>

                      {/* Tools Section */}
                      {serverTools.tools.length > 0 ? (
                        <div className="space-y-3">
                          <h6 className="font-medium text-gray-900 dark:text-gray-100 text-sm">Tools</h6>
                          <div className="space-y-2 max-h-60 overflow-y-auto">
                            {serverTools.tools.map((tool, index) => (
                              <div key={index} className="bg-gray-100 dark:bg-gray-700 p-3 rounded-lg">
                                <div className="flex items-start justify-between">
                                  <div className="flex-1">
                                    <h6 className="font-medium text-gray-900 dark:text-gray-100 text-sm">
                                      {tool.name}
                                    </h6>
                                    <p className="text-xs text-gray-600 dark:text-gray-400 mt-1">
                                      {tool.description}
                                    </p>
                                    
                                    {/* Detailed Parameters */}
                                    {tool.parameters && tool.parameters.properties && (
                                      <div className="mt-2">
                                        <div className="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                                          Parameters:
                                        </div>
                                        <div className="space-y-1">
                                          {Object.entries(tool.parameters.properties || {}).map(([paramName, paramInfo]) => {
                                            const isRequired = tool.parameters.required?.includes(paramName) || false
                                            return (
                                              <div key={paramName} className="text-xs bg-white dark:bg-gray-800 p-2 rounded border">
                                                <div className="flex items-center gap-2 mb-1">
                                                  <span className="font-mono font-medium">{paramName}</span>
                                                  <span className="text-xs bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 px-1 py-0.5 rounded">
                                                    {paramInfo.type || 'string'}
                                                  </span>
                                                  {isRequired && (
                                                    <span className="text-xs bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300 px-1 py-0.5 rounded">
                                                      required
                                                    </span>
                                                  )}
                                                </div>
                                                {paramInfo.description && (
                                                  <p className="text-gray-600 dark:text-gray-400">{paramInfo.description}</p>
                                                )}
                                              </div>
                                            )
                                          })}
                                        </div>
                                      </div>
                                    )}
                                  </div>
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      ) : (
                        <div className="text-center py-4 text-sm text-gray-500 dark:text-gray-400">
                          No tools available
                        </div>
                      )}

                      {/* Prompts Section */}
                      {serverTools.prompts && serverTools.prompts.length > 0 && (
                        <div className="space-y-3">
                          <h6 className="font-medium text-gray-900 dark:text-gray-100 text-sm">Prompts ({serverTools.prompts.length})</h6>
                          <div className="space-y-2 max-h-40 overflow-y-auto">
                            {serverTools.prompts.map((prompt, index) => (
                              <div key={index} className="bg-blue-50 dark:bg-blue-900/20 p-3 rounded-lg border border-blue-200 dark:border-blue-800">
                                <h6 className="font-medium text-blue-900 dark:text-blue-100 text-sm">
                                  {prompt.name}
                                </h6>
                                {prompt.description && (
                                  <p className="text-xs text-blue-700 dark:text-blue-300 mt-1">
                                    {prompt.description}
                                  </p>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* Resources Section */}
                      {serverTools.resources && serverTools.resources.length > 0 && (
                        <div className="space-y-3">
                          <h6 className="font-medium text-gray-900 dark:text-gray-100 text-sm">Resources ({serverTools.resources.length})</h6>
                          <div className="space-y-2 max-h-40 overflow-y-auto">
                            {serverTools.resources.map((resource, index) => (
                              <div key={index} className="bg-green-50 dark:bg-green-900/20 p-3 rounded-lg border border-green-200 dark:border-green-800">
                                <h6 className="font-medium text-green-900 dark:text-green-100 text-sm">
                                  {resource.name}
                                </h6>
                                <p className="text-xs text-green-700 dark:text-green-300 font-mono mt-1">
                                  {resource.uri}
                                </p>
                                {resource.description && (
                                  <p className="text-xs text-green-600 dark:text-green-400 mt-1">
                                    {resource.description}
                                  </p>
                                )}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  )}

                  {!serverTools && !isServerLoading(selectedServer._meta?.["io.modelcontextprotocol.registry/official"]?.serverId || '') && !toolsError && (
                    <div className="text-center py-4 text-sm text-gray-500 dark:text-gray-400">
                      {(() => {
                        const requiredHeaders = getRequiredHeaders(selectedServer)
                        const requiredEnvVars = getRequiredEnvVars(selectedServer)
                        const hasAuth = requiredHeaders.length > 0 || requiredEnvVars.length > 0
                        return hasAuth 
                          ? 'Provide authentication credentials above and click "Load Tools with Auth" to discover available tools.'
                          : 'Click "Load Tools" to discover available tools for this server.'
                      })()}
                    </div>
                  )}
                </div>
              </div>
              
              <div className="flex justify-end gap-2 mt-6">
                <button
                  onClick={() => setSelectedServer(null)}
                  className="px-4 py-2 text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200"
                >
                  Close
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
