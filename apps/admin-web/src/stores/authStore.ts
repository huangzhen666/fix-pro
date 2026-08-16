import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'

interface AuthState {
  authenticated: boolean
  user?: { orgId: number; adminUserId: number; name: string; role: string; platformSuperAdmin?: boolean; mustChangePassword?: boolean }
  permissions: string[]
  setSession: (user: AuthState['user'], permissions?: string[]) => void
  setPermissions: (permissions: string[]) => void
  clearSession: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      authenticated: false,
      permissions: [],
      setSession: (user, permissions = []) => set({ authenticated: true, user, permissions }),
      setPermissions: (permissions) => set({ permissions }),
      clearSession: () => set({ authenticated: false, user: undefined, permissions: [] }),
    }),
    {
      name: 'fixpro-admin-session',
      storage: createJSONStorage(() => sessionStorage),
    },
  ),
)
