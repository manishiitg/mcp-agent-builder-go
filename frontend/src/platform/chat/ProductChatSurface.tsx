import type { CleanConversationSurfaceProps } from '../../components/CleanConversationSurface'
import { CleanConversationSurface } from '../../components/CleanConversationSurface'

/**
 * Canonical product-chat transcript. ChatArea installs this automatically for
 * `inputVariant="product"`, so every current and future product gets the same
 * durable transcript, normalized failures, retry affordance, and safe
 * technical details without reimplementing them in the product package.
 */
export function ProductChatSurface(props: CleanConversationSurfaceProps) {
  return <CleanConversationSurface {...props} />
}
