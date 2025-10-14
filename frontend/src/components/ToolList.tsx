import React from 'react';
import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
// Redefine ToolDefinition type locally
export type ToolDefinition = {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  status?: string;
  error?: string;
  server?: string;
  toolsEnabled?: number;
  function_names?: string[];
};

import { Checkbox } from './ui/checkbox';

interface ToolListProps {
  toolList: ToolDefinition[];
  enabledTools: string[];
  setEnabledTools: (fn: (prev: string[]) => string[]) => void;
  isStreaming: boolean;
  onEdit: (name: string) => void;
  onRemove: (name: string) => void;
  readOnly?: boolean;
}

const ToolList: React.FC<ToolListProps> = ({ toolList, enabledTools, setEnabledTools, isStreaming, onEdit, onRemove, readOnly }) => {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const toggleExpand = (name: string) => {
    setExpanded((prev) => ({ ...prev, [name]: !prev[name] }));
  };

  return (
    <div className="space-y-2 mt-4">
      {toolList.map((tool) => (
        <div key={tool.name} className="flex items-center gap-3 text-sm">
          {/* Status Dot */}
          <span
            className={
              tool.status === 'ok'
                ? 'bg-green-500'
                : tool.status === 'loading'
                ? 'bg-yellow-500'
                : 'bg-red-500'
            }
            style={{
              display: 'inline-block',
              width: 10,
              height: 10,
              borderRadius: '50%',
            }}
            title={
              tool.status === 'ok'
                ? 'Connected'
                : tool.status === 'loading'
                ? 'Loading'
                : 'Not connected'
            }
          />
          {/* Tool/Server Name */}
          <span>{tool.name}</span>
          {/* Function Count & Expand/Collapse */}
          {tool.function_names && tool.function_names.length > 0 && (
            <span className="ml-1 flex items-center gap-1">
              <button
                type="button"
                aria-label={expanded[tool.name] ? 'Collapse function list' : 'Expand function list'}
                onClick={() => toggleExpand(tool.name)}
                className="focus:outline-none"
                style={{ padding: 0, background: 'none', border: 'none', cursor: 'pointer' }}
              >
                {expanded[tool.name] ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
              </button>
              <span className="text-gray-500">{tool.function_names.length} function{tool.function_names.length !== 1 ? 's' : ''}</span>
            </span>
          )}
          {/* Expanded Function Names */}
          {expanded[tool.name] && tool.function_names && tool.function_names.length > 0 && (
            <span
              className="text-sm text-gray-400 ml-1 flex flex-wrap gap-1 max-w-full overflow-x-auto"
              style={{ wordBreak: 'break-all' }}
            >
              [
              {tool.function_names.map((fn) => (
                <code
                  key={fn}
                  className="bg-gray-100 rounded px-1 text-xs"
                  style={{ whiteSpace: 'nowrap' }}
                >
                  {fn}
                </code>
              ))}
              ]
            </span>
          )}
          {/* Status Text */}
          <span className="text-sm text-gray-500">
            {tool.status === 'ok'
              ? (enabledTools.includes(tool.name) ? 'Enabled' : 'Disabled')
              : tool.status === 'loading'
              ? 'Loading tools'
              : 'Not enabled'}
          </span>
          {/* Toggle */}
          {!readOnly && (
            <Checkbox
              checked={enabledTools.includes(tool.name)}
              disabled={tool.status !== 'ok' || isStreaming}
              onCheckedChange={(checked) => {
                if (checked) {
                  setEnabledTools((prev) => [...prev, tool.name]);
                } else {
                  setEnabledTools((prev) => prev.filter((t) => t !== tool.name));
                }
              }}
            />
          )}
          {/* Optional: Error Tooltip */}
          {tool.status === 'error' && tool.error && (
            <span className="text-xs text-red-500" title={tool.error}>!</span>
          )}
          <span className="text-gray-500">{tool.description}</span>
          {!readOnly && (
            <>
              <button
                className="ml-2 text-blue-500 underline"
                onClick={() => onEdit(tool.name)}
              >Edit</button>
              <button
                className="ml-1 text-red-500 underline"
                onClick={() => onRemove(tool.name)}
              >Remove</button>
            </>
          )}
        </div>
      ))}
    </div>
  );
};

export default ToolList; 