import { useEffect, useRef, useState, type ComponentType } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import { RunloopMark } from './branding/RunloopLogo'
import { VideoStudioMark } from '../products/video-studio/VideoStudioMark'
import { ChiefOfStaffMark } from '../products/chief-of-staff/ChiefOfStaffMark'
import { useProductSurfaceStore, type ProductSurface } from '../stores/useProductSurfaceStore'
import { useAppStore } from '../stores/useAppStore'
import { cn } from '../lib/utils'

type ProductSurfaceSwitcherProps = {
  className?: string
  version?: string
}

// RunloopMark/VideoStudioMark render <svg>, ChiefOfStaffMark renders a <div>
// badge (see its own comment) -- this is the common shape all three actually
// need, not svg-specific props.
type ProductMarkComponent = ComponentType<{ className?: string; title?: string }>

const products: Array<{
  id: ProductSurface
  label: string
  description: string
  icon: ProductMarkComponent
}> = [
  { id: 'agentworks', label: 'AgentWorks', description: 'Automation and workflows', icon: RunloopMark },
  { id: 'video-studio', label: 'Video Studio', description: 'Projects and video production', icon: VideoStudioMark },
  { id: 'chief-of-staff', label: 'Chief of Staff', description: 'Your operations hub across automations', icon: ChiefOfStaffMark },
]

export function ProductSurfaceSwitcher({ className, version }: ProductSurfaceSwitcherProps) {
  const productSurface = useProductSurfaceStore((state) => state.productSurface)
  const setProductSurface = useProductSurfaceStore((state) => state.setProductSurface)
  const [open, setOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const currentProduct = products.find((product) => product.id === productSurface) ?? products[0]
  const CurrentIcon = currentProduct.icon

  const activateProduct = (product: ProductSurface) => {
    setOpen(false)
    setProductSurface(product)
    if (product !== 'agentworks') return

    // AgentWorks means Automations now -- Chief of Staff is its own
    // top-level product surface, not a lane restored inside this one.
    const appStore = useAppStore.getState()
    appStore.setModeCategory('workflow')
    appStore.setShowWorkflowsOverview(true)
  }

  useEffect(() => {
    if (!open) return
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  return (
    <div
      ref={menuRef}
      className={cn('relative shrink-0', className)}
    >
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-label="Switch product"
        aria-haspopup="menu"
        aria-expanded={open}
        title={currentProduct.id === 'agentworks' && version ? `${currentProduct.label} v${version}` : currentProduct.label}
        className="flex items-center gap-2.5 rounded-xl border border-slate-200 bg-white px-2 py-1.5 text-left text-slate-900 shadow-sm transition hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:bg-slate-800"
      >
        <CurrentIcon className="h-7 w-7 shrink-0" title="" />
        <span className="whitespace-nowrap text-xs font-semibold">{currentProduct.label}</span>
        <ChevronDown className={`h-3.5 w-3.5 text-slate-400 transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>
      {open ? (
        <div role="menu" aria-label="Products" className="absolute left-0 top-[calc(100%+8px)] z-50 w-64 rounded-2xl border border-slate-200 bg-white p-1.5 shadow-2xl shadow-slate-950/15 dark:border-slate-700 dark:bg-slate-900">
          {products.map((product) => {
            const active = productSurface === product.id
            const Icon = product.icon
            return (
              <button
                key={product.id}
                type="button"
                role="menuitem"
                onClick={() => {
                  activateProduct(product.id)
                }}
                className={cn(
                  'flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors',
                  active
                    ? 'bg-violet-50 dark:bg-violet-950/40'
                    : 'hover:bg-slate-100 dark:hover:bg-slate-800',
                )}
              >
                <Icon className="h-8 w-8 shrink-0" title="" />
                <span className="min-w-0 flex-1">
                  <strong className={cn('block text-xs', active ? 'text-violet-800 dark:text-violet-200' : 'text-slate-900 dark:text-slate-100')}>{product.label}</strong>
                  <small className={cn('mt-0.5 block text-[10px]', active ? 'text-violet-500 dark:text-violet-400' : 'text-slate-400')}>
                    {product.description}
                  </small>
                </span>
                {active ? <Check className="h-4 w-4 shrink-0 text-violet-600 dark:text-violet-300" /> : null}
              </button>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
