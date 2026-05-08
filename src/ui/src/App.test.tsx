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
    vi.stubGlobal('fetch', vi.fn(async (url, opts = {}) => {
      const asString = String(url)
      if (asString.endsWith('/auth/providers')) {
        return { ok: true, status: 200, json: async () => ({ data: { items: [] } }) }
      }
      if (asString.endsWith('/auth/sessions') && opts.method === 'POST') {
        return { ok: true, status: 200, json: async () => ({ data: { token: 'test-token' } }) }
      }
      if (asString.endsWith('/me')) {
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

  it('renders login form', () => {
    renderApp()
    expect(screen.getByLabelText('Login')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('performs login and stores token', async () => {
    renderApp()

    fireEvent.change(screen.getByLabelText('Login'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => {
      expect(localStorage.getItem('homer_v4_token')).toBe('test-token')
    })
    await waitFor(() => {
      expect(screen.getByText('No dashboards available. Create one above or reset to defaults.')).toBeInTheDocument()
    })
  })
})
