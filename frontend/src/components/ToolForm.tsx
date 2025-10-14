import React from 'react';
import type { MCPServerConfig } from '../services/api';
import { Input } from './ui/Input';
import { Button } from './ui/Button';

interface ToolFormProps {
  mode: 'add' | 'edit';
  serverName: string;
  setServerName: (name: string) => void;
  server: MCPServerConfig;
  setServer: (server: MCPServerConfig) => void;
  onSubmit: (name: string, server: MCPServerConfig) => void;
  onCancel?: () => void;
  disabled?: boolean;
}

const ToolForm: React.FC<ToolFormProps> = ({ mode, serverName, setServerName, server, setServer, onSubmit, onCancel, disabled }) => (
  <div>
    <div className="mb-1 font-semibold">{mode === 'add' ? 'Add New Server/Tool' : `Edit: ${serverName}`}</div>
    {mode === 'add' && (
      <Input
        type="text"
        placeholder="Name"
        value={serverName}
        onChange={e => setServerName(e.target.value)}
        className="mb-1 w-full"
        disabled={disabled}
      />
    )}
    <Input
      type="text"
      placeholder="Command"
      value={server.command}
      onChange={e => setServer({ ...server, command: e.target.value })}
      className="mb-1 w-full"
      disabled={disabled}
    />
    <Input
      type="text"
      placeholder="Args (comma separated)"
      value={server.args.join(',')}
      onChange={e => setServer({ ...server, args: e.target.value.split(',') })}
      className="mb-1 w-full"
      disabled={disabled}
    />
    <Input
      type="text"
      placeholder="Description"
      value={server.description}
      onChange={e => setServer({ ...server, description: e.target.value })}
      className="mb-1 w-full"
      disabled={disabled}
    />
    <Button
      className="mr-2"
      onClick={() => onSubmit(serverName, server)}
      disabled={disabled || !serverName || !server.command}
    >{mode === 'add' ? 'Add' : 'Save'}</Button>
    {onCancel && (
      <Button
        variant="secondary"
        onClick={onCancel}
        disabled={disabled}
      >Cancel</Button>
    )}
  </div>
);

export default ToolForm; 