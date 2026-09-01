import { useState, useEffect } from 'react';
import { KeyRound, Plus, Trash2, Eye, EyeOff, Pencil, Globe, Bot } from 'lucide-react';
import { useSecretsStore } from '../../stores';
import { secretsApi } from '../../api/secrets';
import { serverOnlySecretNames } from './secretsManagerUtils';
import type { StoredSecret } from '../../stores';

interface SecretsManagerPanelProps {
  /**
   * The embedded workflow-panel context: stacks the add form above the secret
   * lists instead of sitting beside them, since a side panel is far too narrow
   * for the modal's two-column grid.
   */
  compact?: boolean;
}

// Validate secret name: alphanumeric + underscore only
const isValidName = (name: string) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name);

export default function SecretsManagerPanel({ compact = false }: SecretsManagerPanelProps) {
  const { secrets, addSecret, updateSecret, removeSecret, globalSecrets, fetchGlobalSecrets, storedUserSecrets, botEnabledNames, fetchBotSecrets, toggleBotAccess, lastError, clearLastError } = useSecretsStore();

  // Secrets the server holds that this browser has no local record of.
  const serverOnly = serverOnlySecretNames(storedUserSecrets, secrets);

  useEffect(() => {
    if (globalSecrets.length === 0) {
      fetchGlobalSecrets();
    }
  }, [fetchGlobalSecrets, globalSecrets.length]);

  useEffect(() => {
    fetchBotSecrets();
  }, [fetchBotSecrets]);

  const [newName, setNewName] = useState('');
  const [newValue, setNewValue] = useState('');
  const [isAdding, setIsAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  // Edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [editValue, setEditValue] = useState('');
  const [editLoading, setEditLoading] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);

  // Visibility state
  const [visibleIds, setVisibleIds] = useState<Set<string>>(new Set());
  const [decryptedValues, setDecryptedValues] = useState<Record<string, string>>({});
  const [decryptingIds, setDecryptingIds] = useState<Set<string>>(new Set());

  const handleAdd = async () => {
    setAddError(null);
    const trimmedName = newName.trim().toUpperCase();

    if (!trimmedName) {
      setAddError('Name is required');
      return;
    }
    if (!isValidName(trimmedName)) {
      setAddError('Name must start with a letter or underscore and contain only letters, numbers, and underscores');
      return;
    }
    if (secrets.some(s => s.name === trimmedName)) {
      setAddError(`A secret named "${trimmedName}" already exists`);
      return;
    }
    if (!newValue) {
      setAddError('Value is required');
      return;
    }

    setIsAdding(true);
    try {
      const { encrypted } = await secretsApi.encrypt(newValue);
      addSecret({ name: trimmedName, encryptedValue: encrypted });
      setNewName('');
      setNewValue('');
    } catch (err) {
      setAddError(err instanceof Error ? err.message : 'Failed to encrypt secret');
    } finally {
      setIsAdding(false);
    }
  };

  const handleStartEdit = async (secret: StoredSecret) => {
    setEditingId(secret.id);
    setEditName(secret.name);
    setEditValue('');
    setEditError(null);
    setEditLoading(true);
    try {
      const { value } = await secretsApi.decrypt(secret.encryptedValue);
      setEditValue(value);
    } catch {
      setEditError('Failed to decrypt secret value');
    } finally {
      setEditLoading(false);
    }
  };

  const handleSaveEdit = async () => {
    if (!editingId) return;
    setEditError(null);

    const trimmedName = editName.trim().toUpperCase();
    if (!trimmedName) {
      setEditError('Name is required');
      return;
    }
    if (!isValidName(trimmedName)) {
      setEditError('Invalid name format');
      return;
    }
    if (secrets.some(s => s.name === trimmedName && s.id !== editingId)) {
      setEditError(`A secret named "${trimmedName}" already exists`);
      return;
    }
    if (!editValue) {
      setEditError('Value is required');
      return;
    }

    setEditLoading(true);
    try {
      const { encrypted } = await secretsApi.encrypt(editValue);
      updateSecret(editingId, { name: trimmedName, encryptedValue: encrypted });
      setEditingId(null);
      // Clear cached decrypted value
      setDecryptedValues(prev => {
        const next = { ...prev };
        delete next[editingId];
        return next;
      });
      setVisibleIds(prev => {
        const next = new Set(prev);
        next.delete(editingId);
        return next;
      });
    } catch (err) {
      setEditError(err instanceof Error ? err.message : 'Failed to save secret');
    } finally {
      setEditLoading(false);
    }
  };

  const handleToggleVisibility = async (secret: StoredSecret) => {
    if (visibleIds.has(secret.id)) {
      setVisibleIds(prev => {
        const next = new Set(prev);
        next.delete(secret.id);
        return next;
      });
      return;
    }

    // Decrypt on demand
    if (!decryptedValues[secret.id]) {
      setDecryptingIds(prev => new Set(prev).add(secret.id));
      try {
        const { value } = await secretsApi.decrypt(secret.encryptedValue);
        setDecryptedValues(prev => ({ ...prev, [secret.id]: value }));
      } catch {
        return;
      } finally {
        setDecryptingIds(prev => {
          const next = new Set(prev);
          next.delete(secret.id);
          return next;
        });
      }
    }

    setVisibleIds(prev => new Set(prev).add(secret.id));
  };

  const handleDelete = (id: string) => {
    if (!confirm('Are you sure you want to delete this secret?')) return;
    removeSecret(id);
  };

  return (
    <div className={`min-h-0 flex-1 ${compact ? 'flex flex-col gap-3 overflow-y-auto' : 'grid grid-cols-2 gap-4'}`}>
      {/* Add secret + info */}
      <div className={compact ? 'flex shrink-0 flex-col gap-3' : 'flex flex-col gap-4 overflow-y-auto'}>
        {/* Add New Secret */}
        <div className={`border border-gray-200 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-900 ${compact ? 'p-2.5' : 'p-3'}`}>
          <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Add Secret</h4>
          <div className="space-y-2">
            <input
              type="text"
              placeholder="SECRET_NAME"
              value={newName}
              onChange={(e) => setNewName(e.target.value.toUpperCase())}
              className="w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-amber-500"
            />
            <textarea
              placeholder="Secret value (supports multi-line)"
              value={newValue}
              onChange={(e) => setNewValue(e.target.value)}
              rows={compact ? 2 : 3}
              className="w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-y"
            />
            {addError && (
              <p className="text-xs text-red-500">{addError}</p>
            )}
            <button
              onClick={handleAdd}
              disabled={isAdding}
              className="w-full px-3 py-1.5 text-sm font-medium text-white bg-amber-600 hover:bg-amber-700 disabled:opacity-50 rounded-md transition-colors flex items-center justify-center gap-2"
            >
              <Plus className="w-4 h-4" />
              {isAdding ? 'Encrypting...' : 'Add Secret'}
            </button>
          </div>
        </div>

        {/* Info */}
        <div className="space-y-2 px-1">
          <p className="text-xs text-gray-500 dark:text-gray-400">
            Secrets are encrypted server-side using AES-256 and stored locally in your browser. They are only decrypted at the moment they are injected into a message.
          </p>
          <p className="text-xs text-gray-500 dark:text-gray-400">
            <Bot className="w-3 h-3 inline text-amber-500 mr-0.5" /> <strong className="text-gray-600 dark:text-gray-300">Bot access</strong> — When enabled (amber), the encrypted secret is also stored on the server so bot connectors (Slack, web simulator) can use it. Disabling removes it from the server; the secret remains in your browser only.
          </p>
        </div>
      </div>

      {/* Global secrets + user secrets */}
      <div className={`flex flex-col gap-4 min-h-0 ${compact ? 'shrink-0' : ''}`}>
        {/* Global Secrets (read-only) */}
        {globalSecrets.length > 0 && (
          <div className={`border border-gray-200 dark:border-gray-600 rounded-md bg-gray-50 dark:bg-gray-900 shrink-0 ${compact ? 'p-2.5' : 'p-3'}`}>
            <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2 flex items-center gap-1.5">
              <Globe className="w-3.5 h-3.5 text-blue-500 dark:text-blue-400" />
              Global Secrets
            </h4>
            <p className="text-xs text-gray-500 dark:text-gray-400 mb-2">
              Defined via environment variables. Always included in every query.
            </p>
            <div className="space-y-1.5">
              {globalSecrets.map((gs) => (
                <div
                  key={gs.name}
                  className="flex items-center justify-between px-2 py-1.5 bg-white dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700"
                >
                  <span className="text-sm font-mono font-medium text-gray-900 dark:text-gray-100">
                    {gs.name}
                  </span>
                  <span className="text-xs text-gray-400 font-mono">••••••••</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* User Secrets List */}
        <div className={`flex flex-col min-h-0 ${compact ? 'shrink-0' : 'flex-1'}`}>
          <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2 shrink-0">
            Your Secrets
          </h4>
          <div className={`space-y-2 ${compact ? '' : 'flex-1 overflow-y-auto'}`}>
            {secrets.length === 0 && serverOnly.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-8 text-gray-500 dark:text-gray-400">
                <KeyRound className="w-8 h-8 mb-2 opacity-50" />
                <p className="text-sm font-medium">No secrets stored</p>
                <p className="text-xs mt-1">Add secrets to get started</p>
              </div>
            ) : (
              secrets.map((secret) => (
                <div
                  key={secret.id}
                  className="border border-gray-200 dark:border-gray-600 rounded-md p-2.5 bg-white dark:bg-gray-800"
                >
                  {editingId === secret.id ? (
                    /* Edit Mode */
                    <div className="space-y-2">
                      <input
                        type="text"
                        value={editName}
                        onChange={(e) => setEditName(e.target.value.toUpperCase())}
                        className="w-full px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-amber-500"
                      />
                      <textarea
                        value={editValue}
                        onChange={(e) => setEditValue(e.target.value)}
                        disabled={editLoading}
                        placeholder={editLoading ? 'Decrypting...' : 'Secret value'}
                        rows={3}
                        className="w-full px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-amber-500 resize-y"
                      />
                      {editError && <p className="text-xs text-red-500">{editError}</p>}
                      <div className="flex gap-2">
                        <button
                          onClick={handleSaveEdit}
                          disabled={editLoading}
                          className="px-3 py-1 text-xs font-medium text-white bg-amber-600 hover:bg-amber-700 disabled:opacity-50 rounded transition-colors"
                        >
                          {editLoading ? 'Saving...' : 'Save'}
                        </button>
                        <button
                          onClick={() => setEditingId(null)}
                          className="px-3 py-1 text-xs font-medium text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200 transition-colors"
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    /* Display Mode */
                    <div className="flex items-center justify-between">
                      <div className="flex-1 min-w-0">
                        <div className="text-sm font-mono font-medium text-gray-900 dark:text-gray-100 truncate">
                          {secret.name}
                        </div>
                        <div className="text-xs text-gray-500 dark:text-gray-400 font-mono mt-0.5">
                          {visibleIds.has(secret.id) && decryptedValues[secret.id]
                            ? decryptedValues[secret.id]
                            : '••••••••'
                          }
                        </div>
                      </div>
                      <div className="flex items-center gap-0.5 ml-2 shrink-0">
                        <button
                          onClick={() => handleToggleVisibility(secret)}
                          disabled={decryptingIds.has(secret.id)}
                          className="p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
                          title={visibleIds.has(secret.id) ? 'Hide value' : 'Show value'}
                        >
                          {decryptingIds.has(secret.id) ? (
                            <span className="w-3.5 h-3.5 block animate-pulse text-xs">...</span>
                          ) : visibleIds.has(secret.id) ? (
                            <EyeOff className="w-3.5 h-3.5" />
                          ) : (
                            <Eye className="w-3.5 h-3.5" />
                          )}
                        </button>
                        <button
                          onClick={() => toggleBotAccess(secret.id)}
                          className={`p-1 transition-colors ${
                            botEnabledNames.has(secret.name)
                              ? 'text-amber-500 hover:text-amber-600 dark:text-amber-400 dark:hover:text-amber-300'
                              : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                          }`}
                          title={botEnabledNames.has(secret.name) ? 'Available to bots' : 'Not available to bots'}
                        >
                          <Bot className="w-3.5 h-3.5" />
                        </button>
                        <div className="w-px h-3.5 bg-gray-300 dark:bg-gray-600 mx-0.5" />
                        <button
                          onClick={() => handleStartEdit(secret)}
                          className="p-1 text-gray-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
                          title="Edit secret"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => handleDelete(secret.id)}
                          className="p-1 text-gray-400 hover:text-red-600 dark:hover:text-red-400 transition-colors"
                          title="Delete secret"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              ))
            )}
            {lastError ? (
              <div role="alert" data-testid="secrets-save-error" className="rounded-md border border-red-300 bg-red-50 p-2.5 text-xs text-red-700 dark:border-red-800 dark:bg-red-950/40 dark:text-red-300">
                <div className="flex items-start gap-2">
                  <span className="flex-1">{lastError}</span>
                  <button type="button" onClick={clearLastError} className="shrink-0 underline">Dismiss</button>
                </div>
              </div>
            ) : null}
            {serverOnly.map((name) => (
              <div
                key={`server:${name}`}
                data-testid="server-only-secret"
                className="border border-gray-200 dark:border-gray-600 rounded-md p-2.5 bg-gray-50 dark:bg-gray-900/40"
              >
                <div className="flex items-center gap-2">
                  <KeyRound className="w-3.5 h-3.5 shrink-0 text-gray-400" />
                  <span className="font-mono text-sm text-gray-900 dark:text-gray-100 truncate">{name}</span>
                  <span className="ml-auto shrink-0 rounded bg-gray-200 dark:bg-gray-700 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:text-gray-300">
                    Saved on server
                  </span>
                </div>
                <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Available to agents in this workspace. Its value was saved elsewhere, so it cannot be shown or edited here — add it again above to replace it.
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
