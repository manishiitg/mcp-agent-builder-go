import { useCallback, useEffect, useState } from 'react';
import { CheckCircle2, Eye, EyeOff, KeyRound, Loader2, Trash2 } from 'lucide-react';
import { Button } from './ui/Button';
import { secretsApi, type WorkflowCredentialProvider } from '../api/secrets';
import { useChatStore } from '../stores/useChatStore';
import { READ_ONLY_TITLE } from '../hooks/useCanWriteWorkflow';
import { maskCredentialPreviewClient } from '../utils/maskCredentialPreview';


export interface WorkflowProviderCredentialCopy {
  /** Section heading, e.g. "Claude Code login for this automation". */
  heading: string;
  /** One-line explanation of where the credential comes from. */
  hint: React.ReactNode;
  /** Shown in the status chip when nothing is stored. */
  fallbackLabel: string;
  inputPlaceholder: string;
  replacePlaceholder: string;
  /** Noun used in buttons and toasts, e.g. "token" or "API key". */
  noun: string;
  /** Toast shown after a successful delete. */
  removedMessage: string;
  savedMessage: string;
}

interface WorkflowProviderCredentialFieldProps {
  provider: WorkflowCredentialProvider;
  copy: WorkflowProviderCredentialCopy;
  /**
   * Workflow this credential belongs to. Undefined while the automation is
   * still a draft — a draft has no stable path to scope the credential to, so
   * the field explains that instead of offering to save.
   */
  workflowCredentialPath?: string;
  /** Disables the controls while the surrounding form is submitting. */
  isFormBusy?: boolean;
  /** The user lacks write access: the input, save, and remove all disable. */
  readOnly?: boolean;
  /** Resets the entry field whenever the parent form is re-opened or switched. */
  resetKey?: string;
  inputId: string;
  /**
   * Reports whether the field holds credential text the user has not saved yet.
   * The parent blocks its own submit on this: saving the automation while a
   * pasted credential sits unsaved in the box looks like it stored the
   * credential, and it does not.
   */
  onDirtyChange?: (dirty: boolean) => void;
}

/**
 * Per-workflow credential entry for a coding-CLI provider.
 *
 * Claude Code and Cursor share this because they solve the same problem the
 * same way: without a scoped credential both fall back to whichever login is on
 * the server, silently billing that account. Keeping one implementation is what
 * makes "same as Claude Code" true rather than aspirational — a fix to the
 * masked preview, the busy-session conflict, or the error surfacing lands on
 * both providers at once.
 */
export function WorkflowProviderCredentialField({
  provider,
  copy,
  workflowCredentialPath,
  isFormBusy = false,
  readOnly = false,
  resetKey,
  inputId,
  onDirtyChange,
}: WorkflowProviderCredentialFieldProps) {
  const [value, setValue] = useState('');
  const [configured, setConfigured] = useState(false);
  // Masked "first4...last4" preview of the saved credential. The server
  // decrypts to build this and never returns the full value — it lets a user
  // confirm which credential is saved without re-exposing it.
  const [preview, setPreview] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isRevealed, setIsRevealed] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setValue('');
    setIsRevealed(false);
    setError(null);
  }, [resetKey]);

  useEffect(() => {
    onDirtyChange?.(value.trim() !== '');
  }, [onDirtyChange, value]);

  useEffect(() => {
    if (!workflowCredentialPath) {
      setConfigured(false);
      setPreview(null);
      setIsLoading(false);
      return;
    }
    let cancelled = false;
    setIsLoading(true);
    void secretsApi.getWorkflowProviderCredentialStatus(provider, workflowCredentialPath)
      .then(status => {
        if (!cancelled) {
          setConfigured(status.configured);
          setPreview(status.preview ?? null);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setConfigured(false);
          setPreview(null);
        }
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => { cancelled = true; };
  }, [provider, workflowCredentialPath]);

  const handleSave = useCallback(async () => {
    const trimmed = value.trim();
    if (!workflowCredentialPath || !trimmed) return;
    setIsSaving(true);
    setError(null);
    try {
      await secretsApi.storeWorkflowProviderCredential(provider, workflowCredentialPath, trimmed);
      setConfigured(true);
      setPreview(maskCredentialPreviewClient(trimmed));
      setValue('');
      setIsRevealed(false);
      useChatStore.getState().addToast(copy.savedMessage, 'success');
    } catch (err) {
      // The server returns its rejection reason as a plain-text body — it
      // distinguishes expired from revoked from mistyped, which a generic
      // message would throw away.
      const serverDetail = (err as { response?: { data?: unknown } })?.response?.data;
      const detail = typeof serverDetail === 'string' && serverDetail.trim() !== ''
        ? serverDetail.trim()
        : err instanceof Error ? err.message : `Unable to save this ${copy.noun}`;
      setError(detail);
      useChatStore.getState().addToast(`Failed to save ${copy.noun}: ${detail}`, 'error');
    } finally {
      setIsSaving(false);
    }
  }, [copy.noun, copy.savedMessage, provider, value, workflowCredentialPath]);

  const handleDelete = useCallback(async () => {
    if (!workflowCredentialPath) return;
    setIsDeleting(true);
    try {
      await secretsApi.deleteWorkflowProviderCredential(provider, workflowCredentialPath);
      setConfigured(false);
      setPreview(null);
      setValue('');
      useChatStore.getState().addToast(copy.removedMessage, 'success');
    } catch (err) {
      const detail = err instanceof Error ? err.message : 'Unknown error';
      useChatStore.getState().addToast(`Failed to remove ${copy.noun}: ${detail}`, 'error');
    } finally {
      setIsDeleting(false);
    }
  }, [copy.noun, copy.removedMessage, provider, workflowCredentialPath]);

  return (
    <div>
      <label className="mb-2 flex items-center gap-2 text-sm font-medium">
        <KeyRound className="h-4 w-4" />
        {copy.heading}
      </label>
      <div className="rounded-md border border-border bg-muted/30 p-3 text-foreground">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-xs leading-relaxed text-muted-foreground">{copy.hint}</p>
          <span className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
            {isLoading ? (
              <><Loader2 className="h-3.5 w-3.5 animate-spin" /> Checking</>
            ) : configured ? (
              <>
                <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" /> Saved
                {preview && (
                  <code
                    className="rounded bg-background px-1 py-0.5 font-mono text-foreground"
                    title="Masked preview — the full credential is never sent to the browser"
                  >
                    {preview}
                  </code>
                )}
              </>
            ) : (
              copy.fallbackLabel
            )}
          </span>
        </div>

        {workflowCredentialPath ? (
          <div className="mt-3 flex flex-col gap-2 sm:flex-row">
            <div className="relative min-w-0 flex-1">
              <input
                id={inputId}
                type={isRevealed ? 'text' : 'password'}
                autoComplete="off"
                value={value}
                onChange={event => {
                  setValue(event.target.value);
                  setError(null);
                }}
                placeholder={configured ? copy.replacePlaceholder : copy.inputPlaceholder}
                disabled={readOnly}
                className="h-9 w-full rounded-md border border-input bg-background px-3 pr-10 font-mono text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring"
              />
              <button
                type="button"
                onClick={() => setIsRevealed(previous => !previous)}
                className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                aria-label={isRevealed ? `Hide ${copy.noun}` : `Show ${copy.noun}`}
                title={isRevealed ? `Hide ${copy.noun}` : `Show ${copy.noun}`}
              >
                {isRevealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            <Button
              type="button"
              size="sm"
              onClick={handleSave}
              disabled={readOnly || !value.trim() || isSaving || isFormBusy}
              title={readOnly ? READ_ONLY_TITLE : undefined}
              className="h-9 shrink-0"
            >
              {isSaving && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
              {isSaving ? 'Verifying...' : `Save ${copy.noun}`}
            </Button>
          </div>
        ) : (
          <p className="mt-3 text-xs text-muted-foreground">
            Create the automation first, then edit it to add a workflow-specific {copy.noun}.
          </p>
        )}

        {error && (
          <p role="alert" className="mt-2 text-xs text-red-600 dark:text-red-400">{error}</p>
        )}

        <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
          <span className="text-[11px] text-muted-foreground">
            This {copy.noun} is private to the current user and automation.
          </span>
          {configured && (
            <button
              type="button"
              onClick={handleDelete}
              disabled={readOnly || isDeleting || isSaving || isFormBusy}
              title={readOnly ? READ_ONLY_TITLE : undefined}
              className="inline-flex shrink-0 items-center gap-1 text-xs text-red-600 hover:text-red-700 disabled:opacity-50 dark:text-red-400"
            >
              {isDeleting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
              Remove {copy.noun}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export default WorkflowProviderCredentialField;
