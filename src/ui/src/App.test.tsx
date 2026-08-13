import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import App from './App'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ConfirmProvider } from '@/components/ui/confirm-dialog'

function renderApp() {
  return render(
    <TooltipProvider delayDuration={200}>
      <ConfirmProvider>
        <App />
      </ConfirmProvider>
    </TooltipProvider>,
  )
}

describe('App smoke/integration', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    let sessionActive = false
    vi.stubGlobal('fetch', vi.fn(async (url, opts = {}) => {
      const asString = String(url)
      if (asString.endsWith('/auth/providers')) {
        return { ok: true, status: 200, json: async () => ({ data: { internal: { enable: true } } }) }
      }
      if (asString.endsWith('/auth/sessions') && opts.method === 'POST') {
        sessionActive = true
        return { ok: true, status: 201, json: async () => ({ data: { token: 'test-token' } }) }
      }
      if (asString.endsWith('/me')) {
        if (!sessionActive) {
          return { ok: false, status: 401, json: async () => ({}) }
        }
        return { ok: true, status: 200, json: async () => ({ data: { username: 'tester', admin: true } }) }
      }
      if (asString.endsWith('/dashboards')) {
        return { ok: true, status: 200, json: async () => ({ data: { items: [] } }) }
      }
      return { ok: true, status: 200, json: async () => ({ data: {} }) }
    }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders login form', async () => {
    renderApp()
    await waitFor(() => {
      expect(screen.getByLabelText('Login')).toBeInTheDocument()
    })
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('performs login without persisting JWT by default (cookie session)', async () => {
    renderApp()
    await waitFor(() => {
      expect(screen.getByLabelText('Login')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Login'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => {
      expect(localStorage.getItem('homer_v4_token')).toBeNull()
      expect(screen.getByText('No dashboards available. Create one above or reset to defaults.')).toBeInTheDocument()
    })
  })

  it('defaults to system theme and follows prefers-color-scheme when storage is empty', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: String(query).includes('prefers-color-scheme: dark') ? false : false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    )

    renderApp()
    await waitFor(() => {
      expect(screen.getByLabelText('Login')).toBeInTheDocument()
    })
    expect(document.documentElement.classList.contains('light')).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })
})
