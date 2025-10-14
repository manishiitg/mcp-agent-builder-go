import { RefreshCw, Search, X } from 'lucide-react'

interface WorkspaceHeaderProps {
  loading: boolean
  onRefresh: () => void
  searchQuery: string
  onSearchChange: (query: string) => void
}

export default function WorkspaceHeader({ loading, onRefresh, searchQuery, onSearchChange }: WorkspaceHeaderProps) {
  return (
    <>
      {/* Header - Same height as other sections */}
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 h-16 flex items-center">
        <div className="flex items-center justify-between w-full">
          <div>
            <h2 className="text-base font-semibold text-gray-900 dark:text-gray-100">
              Workspace
            </h2>
            <p className="text-xs text-gray-600 dark:text-gray-400">
              Browse files and send them to chat context
            </p>
          </div>
          <button
            onClick={onRefresh}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 disabled:opacity-50"
            title="Refresh files"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>
      
      {/* Search/Filter Input - Separate section below header */}
      <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700">
        <div className="relative">
          <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <Search className="h-4 w-4 text-gray-400" />
          </div>
          <input
            type="text"
            placeholder="Search files and folders..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            className="block w-full pl-10 pr-10 py-2 border border-gray-300 dark:border-gray-600 rounded-md leading-5 bg-white dark:bg-gray-800 placeholder-gray-500 dark:placeholder-gray-400 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500 text-sm"
          />
          {searchQuery && (
            <div className="absolute inset-y-0 right-0 pr-3 flex items-center">
              <button
                onClick={() => onSearchChange('')}
                className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                title="Clear search"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          )}
        </div>
      </div>
    </>
  )
}
