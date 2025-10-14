import { useState } from 'react';
import { Server, ChevronDown, Check } from 'lucide-react';
import { Button } from './ui/Button';
import { Checkbox } from './ui/checkbox';
import { Card } from './ui/Card';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip';

interface ServerSelectionDropdownProps {
  availableServers: string[];
  selectedServers: string[];
  onServerToggle: (server: string) => void;
  onSelectAll: () => void;
  onClearAll: () => void;
  disabled?: boolean;
}

export default function ServerSelectionDropdown({
  availableServers,
  selectedServers,
  onServerToggle,
  onSelectAll,
  onClearAll,
  disabled = false
}: ServerSelectionDropdownProps) {
  const [isOpen, setIsOpen] = useState(false);

  const handleServerToggle = (server: string) => {
    onServerToggle(server);
  };

  const getDisplayText = () => {
    if (selectedServers.length === 0) {
      return `All servers (${availableServers.length})`;
    } else if (selectedServers.length === availableServers.length) {
      return `All servers (${availableServers.length})`;
    } else if (selectedServers.length === 1) {
      return selectedServers[0];
    } else {
      return `${selectedServers.length} servers`;
    }
  };

  const isAllSelected = selectedServers.length === availableServers.length;
  const isNoneSelected = selectedServers.length === 0;

  return (
    <TooltipProvider>
      <div className="relative">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIsOpen(!isOpen)}
              disabled={disabled || availableServers.length === 0}
              className="h-8 px-2 text-xs font-medium bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-700"
            >
              <Server className="w-3 h-3 mr-1" />
              {getDisplayText()}
              <ChevronDown className="w-3 h-3 ml-1" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            <p>{availableServers.length === 0 ? 'No MCP servers available' : 'Select MCP servers to use'}</p>
          </TooltipContent>
        </Tooltip>

        {isOpen && (
          <>
            {/* Backdrop */}
            <div 
              className="fixed inset-0 z-40" 
              onClick={() => setIsOpen(false)}
            />
            
            {/* Dropdown */}
            <div className="absolute bottom-full left-0 mb-1 z-50 min-w-[280px]">
              <Card className="p-4 shadow-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
                <div className="space-y-3">
                  {/* Header */}
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">
                      Select Servers
                    </h3>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setIsOpen(false)}
                      className="h-6 w-6 p-0 text-gray-400 hover:text-gray-600"
                    >
                      ✕
                    </Button>
                  </div>

                  {/* Quick Actions */}
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        onSelectAll();
                      }}
                      disabled={isAllSelected}
                      className="h-7 px-2 text-xs"
                    >
                      All
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        onClearAll();
                      }}
                      disabled={isNoneSelected}
                      className="h-7 px-2 text-xs"
                    >
                      None
                    </Button>
                  </div>

                  {/* Server List */}
                  <div className="max-h-48 overflow-y-auto space-y-2 border rounded-md p-2">
                    {availableServers.length > 0 ? (
                      availableServers.map((server) => (
                        <div key={server} className="flex items-center space-x-2">
                          <Checkbox
                            id={`manual-server-${server}`}
                            checked={selectedServers.includes(server)}
                            onCheckedChange={() => handleServerToggle(server)}
                            className="h-4 w-4"
                          />
                          <label
                            htmlFor={`manual-server-${server}`}
                            className="text-sm cursor-pointer flex-1 text-gray-900 dark:text-gray-100"
                          >
                            {server}
                          </label>
                          {selectedServers.includes(server) && (
                            <Check className="w-3 h-3 text-green-600" />
                          )}
                        </div>
                      ))
                    ) : (
                      <div className="text-sm text-gray-500 text-center py-4">
                        No servers available. Make sure MCP servers are running and connected.
                      </div>
                    )}
                  </div>

                  {/* Selection Summary */}
                  {selectedServers.length > 0 && selectedServers.length < availableServers.length && (
                    <div className="text-xs text-gray-500 bg-gray-50 dark:bg-gray-700 rounded p-2">
                      Selected: {selectedServers.join(', ')}
                    </div>
                  )}

                  {/* Instructions */}
                  <div className="text-xs text-gray-500">
                    {availableServers.length === 0 
                      ? 'No servers available - check MCP server connections'
                      : selectedServers.length === 0 
                        ? 'No servers selected - all servers will be used'
                        : `${selectedServers.length} of ${availableServers.length} servers selected`
                    }
                  </div>
                </div>
              </Card>
            </div>
          </>
        )}
      </div>
    </TooltipProvider>
  );
}
