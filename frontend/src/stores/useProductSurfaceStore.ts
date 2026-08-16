import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ProductSurface = 'agentworks' | 'video-studio' | 'chief-of-staff'

interface ProductSurfaceState {
  productSurface: ProductSurface
  lastVideoProjectId: string | null
  setProductSurface: (surface: ProductSurface) => void
  setLastVideoProjectId: (projectId: string | null) => void
}

export const useProductSurfaceStore = create<ProductSurfaceState>()(
  persist(
    (set) => ({
      productSurface: 'agentworks',
      lastVideoProjectId: null,
      setProductSurface: (productSurface) => set({ productSurface }),
      setLastVideoProjectId: (lastVideoProjectId) => set({ lastVideoProjectId }),
    }),
    {
      name: 'agentworks-product-surface',
    },
  ),
)
