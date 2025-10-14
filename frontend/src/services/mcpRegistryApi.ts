// MCP Registry API service using backend proxy
// Backend handles CORS and proxies requests to https://registry.modelcontextprotocol.io/v0
const MCP_REGISTRY_BASE_URL = 'http://localhost:8000/api/mcp-registry'

export interface MCPRegistryServer {
  $schema?: string;
  name: string;
  description: string;
  version: string;
  status?: string;
  websiteUrl?: string;
  repository?: {
    id: string;
    url: string;
    source: string;
    subfolder?: string;
  };
  packages?: Array<{
    registryType: string;
    registryBaseUrl?: string;
    identifier: string;
    version: string;
    fileSha256?: string;
    runtimeHint?: string;
    transport: {
      type: string;
      url?: string;
      headers?: Array<{
        name: string;
        description: string;
        isRequired?: boolean;
        isSecret?: boolean;
        default?: string;
        format?: string;
        choices?: string[];
        value?: string;
        variables?: Record<string, unknown>;
      }>;
    };
    environmentVariables?: Array<{
      name: string;
      description: string;
      isRequired?: boolean;
      isSecret?: boolean;
      default?: string;
      format?: string;
      choices?: string[];
      value?: string;
      variables?: Record<string, unknown>;
    }>;
    packageArguments?: Array<{
      name: string;
      description: string;
      type: string;
      isRequired?: boolean;
      isRepeated?: boolean;
      isSecret?: boolean;
      default?: string;
      format?: string;
      choices?: string[];
      value?: string;
      valueHint?: string;
      variables?: Record<string, unknown>;
    }>;
    runtimeArguments?: Array<{
      name: string;
      description: string;
      type: string;
      isRequired?: boolean;
      isRepeated?: boolean;
      isSecret?: boolean;
      default?: string;
      format?: string;
      choices?: string[];
      value?: string;
      valueHint?: string;
      variables?: Record<string, unknown>;
    }>;
  }>;
  remotes?: Array<{
    type: string;
    url: string;
    headers?: Array<{
      name: string;
      description: string;
      isRequired?: boolean;
      isSecret?: boolean;
      default?: string;
      format?: string;
      choices?: string[];
      value?: string;
      variables?: Record<string, unknown>;
    }>;
  }>;
  _meta?: {
    "io.modelcontextprotocol.registry/official": {
      serverId: string;
      versionId: string;
      publishedAt: string;
      updatedAt: string;
      isLatest: boolean;
    };
    "io.modelcontextprotocol.registry/publisher-provided"?: Record<string, unknown>;
  };
}

export interface MCPRegistrySearchParams {
  query?: string;
  limit?: number;
  cursor?: string;
}

export interface CacheStatus {
  isCached: boolean;
  toolsCount?: number;
  promptsCount?: number;
  resourcesCount?: number;
  lastUpdated?: string;
}

export interface EnhancedMCPRegistryServer extends MCPRegistryServer {
  cacheStatus?: CacheStatus;
}

export interface MCPRegistryResponse {
  servers: EnhancedMCPRegistryServer[];
  metadata: {
    next_cursor: string;
    count: number;
  };
}

// Tool-related interfaces (matching backend ToolStatus format)
export interface ParameterSchema {
  type?: string;
  description?: string;
  default?: unknown;
  enum?: string[];
  [key: string]: unknown;
}

export interface ToolParameters {
  type?: string;
  properties?: Record<string, ParameterSchema>;
  required?: string[];
}

export interface ToolDetail {
  name: string;
  description: string;
  parameters: ToolParameters;
  required?: string[];
}

export interface PromptDetail {
  name: string;
  description?: string;
}

export interface ResourceDetail {
  name: string;
  uri: string;
  description?: string;
}

export interface RegistryServerTools {
  name: string;
  server: string;
  status: string;
  description: string;
  toolsEnabled: number;
  functionNames: string[];
  tools: ToolDetail[];
  prompts?: PromptDetail[];
  resources?: ResourceDetail[];
}


export const mcpRegistryApi = {
  // Search servers from registry via backend proxy
  searchServers: async (params: MCPRegistrySearchParams = {}): Promise<MCPRegistryResponse> => {
    try {
      const searchParams = new URLSearchParams()
      if (params.query) searchParams.append('search', params.query)
      if (params.limit) searchParams.append('limit', params.limit.toString())
      if (params.cursor) searchParams.append('cursor', params.cursor)
      
      const response = await fetch(`${MCP_REGISTRY_BASE_URL}/servers?${searchParams}`)
      
      if (!response.ok) {
        throw new Error(`Backend API error: ${response.status} ${response.statusText}`)
      }
      
      const data: MCPRegistryResponse = await response.json()
      return data
    } catch (error) {
      console.error('Failed to search MCP servers:', error)
      throw error
    }
  },

  // Get server details by ID via backend proxy
  getServerDetails: async (serverId: string): Promise<MCPRegistryServer> => {
    try {
      const response = await fetch(`${MCP_REGISTRY_BASE_URL}/servers/${serverId}`)
      
      if (!response.ok) {
        throw new Error(`Backend API error: ${response.status} ${response.statusText}`)
      }
      
      return await response.json()
    } catch (error) {
      console.error('Failed to get server details:', error)
      throw error
    }
  },

  // Get server tools by ID via backend proxy
  getServerTools: async (serverId: string, authConfig?: { headers?: Record<string, string>, envVars?: Record<string, string> }): Promise<RegistryServerTools> => {
    try {
      const response = await fetch(`${MCP_REGISTRY_BASE_URL}/servers/${serverId}/tools`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ 
          headers: authConfig?.headers || {},
          envVars: authConfig?.envVars || {}
        })
      })
      
      if (!response.ok) {
        // Try to get the actual error message from response body
        let errorMessage = `${response.status} ${response.statusText}`
        try {
          const errorText = await response.text()
          if (errorText) {
            errorMessage = errorText
          }
        } catch {
          // If we can't read the body, use status text
        }
        throw new Error(`Backend API error: ${errorMessage}`)
      }
      
      return await response.json()
    } catch (error) {
      console.error('Failed to get server tools:', error)
      throw error
    }
  },


  // Convert registry server to MCPServerConfig
  convertToServerConfig: (server: MCPRegistryServer): import('./api-types').MCPServerConfig => {
    // Extract installation info from packages or remotes
    let command = 'npx'
    let args: string[] = []
    const env: Record<string, string> = {}
    
    if (server.packages && server.packages.length > 0) {
      const pkg = server.packages[0]
      if (pkg.registryType === 'npm') {
        command = 'npx'
        args = [pkg.identifier]
      } else if (pkg.registryType === 'pypi') {
        command = 'pip'
        args = ['install', pkg.identifier]
      }
      
      // Extract environment variables
      if (pkg.environmentVariables) {
        pkg.environmentVariables.forEach(envVar => {
          env[envVar.name] = envVar.description
        })
      }
    }
    
    return {
      command,
      args,
      env,
      description: server.description
    }
  }
}
