// The API the app uses. Swapping the backend is a one-line change here.
import type { FamilyApi } from './familyApi'
import { standaloneApi } from './standaloneApi'

export const api: FamilyApi = standaloneApi
export type { FamilyApi } from './familyApi'
