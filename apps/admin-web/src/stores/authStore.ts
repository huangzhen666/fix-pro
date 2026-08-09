import { create } from 'zustand'
import { createJSONStorage, persist } from 'zustand/middleware'

interface AuthState {
  credential?: string
  setBasicCredential: (username: string, password: string) => void
  clearCredential: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      credential: undefined,
      setBasicCredential: (username, password) => {
        set({ credential: window.btoa(`${username}:${password}`) })
      },
      clearCredential: () => set({ credential: undefined }),
    }),
    {
      name: 'fixpro-bootstrap-auth',
      storage: createJSONStorage(() => sessionStorage),
    },
  ),
)
