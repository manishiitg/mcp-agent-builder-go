import { useEffect } from 'react'
import LearningApp from './LearningApp'
import { installVoiceLifecycle } from './platform/voiceLifecycle'

/**
 * SparkQuill as a product surface of the main frontend, mounted by App.tsx
 * when the deployment's (or the user's) product surface is `sparkquill`.
 *
 * The learning app used to be its own Vite root (frontend/learning-app) with
 * its own entry; docs/design/sparkquill_desktop_on_platform_plan.md (P2b)
 * folds it into this build so there is one frontend to maintain. What that
 * entry did besides rendering — the desktop shell's window-visibility hook
 * for the speech model — is the one side effect left here.
 */
export function SparkQuillSurface() {
  useEffect(() => { installVoiceLifecycle() }, [])
  return <LearningApp />
}
