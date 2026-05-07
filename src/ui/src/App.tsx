import { useEffect, useMemo, useRef, useState } from 'react'
import './App.css'
import AboutPanel from './settings/AboutPanel'
import ProfilePanel from './settings/ProfilePanel'
import DashboardPanel from './settings/DashboardPanel'
import { SettingsSidebar } from './settings/SettingsSidebar'
import UsersPanel from './settings/UsersPanel'
import AliasesPanel from './settings/AliasesPanel'
import AdvancedPanel from './settings/AdvancedPanel'
import MappingsPanel from './settings/MappingsPanel'
import HepsubsPanel from './settings/HepsubsPanel'
import AuthTokensPanel from './settings/AuthTokensPanel'
import AgentSubsPanel from './settings/AgentSubsPanel'
import SystemPanel from './settings/SystemPanel'
import DashboardsPanel from './settings/DashboardsPanel'
import UserSettingsPanel from './settings/UserSettingsPanel'
import ScriptsPanel from './settings/ScriptsPanel'
import ResetPanel from './settings/ResetPanel'
import ApiDocsPanel from './settings/ApiDocsPanel'
import { detectRole, getSectionPerms, canViewSection, canWriteSection } from './settings/permissions'
import DashboardHeader from './dashboard/DashboardHeader'
import { handleUnauthorized } from './api'
import { ThemeProvider } from "@/components/theme/theme-provider"
import { WindowDock } from "@/components/ui/window-dock"
import { useConfirm } from "@/components/ui/confirm-dialog"
import { LoginPage } from './LoginPage'
import { parseLoginProvidersPayload, type PasswordAuthMethodRow } from './loginProviders'
import { toast } from 'sonner'


const apiBase = import.meta.env.VITE_API_BASE || '/api/v4'
const tokenKey = 'homer_v4_token'

function App() {
  const confirm = useConfirm()
  const [token, setToken] = useState(() => localStorage.getItem(tokenKey) || '')
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [activeSection, setActiveSection] = useState('about')
  const [me, setMe] = useState<any>(null)
  const [users, setUsers] = useState<any[]>([])
  const [loadingMe, setLoadingMe] = useState(false)
  const [loadingUsers, setLoadingUsers] = useState(false)
  const [savingUser, setSavingUser] = useState(false)
  const [deletingUser, setDeletingUser] = useState('')
  const [editingUser, setEditingUser] = useState<any>(null)
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [userForm, setUserForm] = useState<any>({
    username: '',
    name: '',
    email: '',
    password: '',
    user_group: 'user',
    enabled: true,
  })
  const [filters, setFilters] = useState<any>({
    search: '',
    username: '',
    email: '',
    is_admin: '',
    enabled: '',
    limit: '50',
  })
  /** False while not on Users settings; used to load immediately on enter and debounce filter changes. */
  const usersSettingsActiveRef = useRef(false)
  const [passwordAuthMethods, setPasswordAuthMethods] = useState<PasswordAuthMethodRow[]>([])
  const [oauthProviderNames, setOauthProviderNames] = useState<string[]>([])
  const [autoOAuthProvider, setAutoOAuthProvider] = useState<string | null>(null)
  const role = useMemo(() => detectRole(me), [me])
  const usersPerms = useMemo(() => getSectionPerms(role, 'users'), [role])

  useEffect(() => {
    if (token) {
      localStorage.setItem(tokenKey, token)
    } else {
      localStorage.removeItem(tokenKey)
    }
  }, [token])

  // One-shot migration from legacy 'theme' key to shadcn ThemeProvider's 'vite-ui-theme' key.
  useEffect(() => {
    const legacy = localStorage.getItem('theme')
    if (legacy && !localStorage.getItem('vite-ui-theme')) {
      localStorage.setItem('vite-ui-theme', legacy)
    }
    if (legacy) localStorage.removeItem('theme')
  }, [])

  useEffect(() => {
    if (!token) {
      loadAuthProviders()
      return
    }

    const applyHash = () => {
      const hash = window.location.hash.replace('#', '')
      if (hash === 'settings') {
        openSettings()
      } else if (hash === 'dashboard' || hash === '') {
        setSettingsOpen(false)
      }
    }

    applyHash()
    window.addEventListener('hashchange', applyHash)
    return () => window.removeEventListener('hashchange', applyHash)
  }, [token])

  useEffect(() => {
    if (!settingsOpen) return
    if (canViewSection(role, activeSection)) return
    const fallback = ['profile', 'about', 'users', 'advanced', 'reset'].find((key) =>
      canViewSection(role, key),
    )
    if (fallback) setActiveSection(fallback)
  }, [settingsOpen, role, activeSection])

  const authHeader = useMemo<Record<string, string>>(() => {
    if (!token) return {} as Record<string, string>
    return { Authorization: `Bearer ${token}` }
  }, [token])

  const parseError = async (res) => {
    let detail = `Request failed (${res.status})`
    try {
      const payload = await res.json()
      if (payload?.error?.detail) {
        detail = payload.error.detail
      } else if (payload?.error?.title) {
        detail = payload.error.title
      }
    } catch {
      // ignore
    }
    return detail
  }

  const loadAuthProviders = async () => {
    try {
      const res = await fetch(`${apiBase}/auth/providers`)
      if (res.ok) {
        const payload = await res.json()
        const parsed = parseLoginProvidersPayload(payload)
        setPasswordAuthMethods(parsed.passwordMethods)
        setOauthProviderNames(parsed.oauthProviderNames)
        setAutoOAuthProvider(parsed.autoOAuthProvider)
      }
    } catch {
      // ignore - providers are optional
    }
  }

  const loginOAuth2 = (provider: string) => {
    window.location.href = `${apiBase}/auth/oauth2/${encodeURIComponent(provider)}/redirect`
  }

  const logout = () => {
    setToken('')
    setMe(null)
    setUsers([])
    setSettingsOpen(false)
    setActiveSection('about')
    toast.success('Logged out')
  }

  const openSettings = () => {
    setSettingsOpen(true)
    setActiveSection('about')
    if (window.location.hash !== '#settings') {
      window.location.hash = '#settings'
    }
  }

  const openDashboard = () => {
    setSettingsOpen(false)
    if (window.location.hash !== '#dashboard') {
      window.location.hash = '#dashboard'
    }
  }

  const loadMe = async () => {
    if (!token) return
    setLoadingMe(true)
    try {
      const res = await fetch(`${apiBase}/me`, { headers: authHeader })
      if (res.status === 401) {
        handleUnauthorized()
        return
      }
      if (!res.ok) {
        const message = await parseError(res)
        toast.error(message)
        return
      }
      const payload = await res.json()
      setMe(payload?.data || null)
    } catch (err) {
      toast.error(`Failed to load profile: ${(err as Error).message}`)
    } finally {
      setLoadingMe(false)
    }
  }

  const buildUsersQuery = () => {
    const params = new URLSearchParams()
    if (filters.limit !== undefined && filters.limit !== '') {
      const lim = parseInt(String(filters.limit), 10)
      if (Number.isFinite(lim) && lim > 0) params.append('page[limit]', String(lim))
    }
    // Backend: parseUserListFilters reads plain `search`, not filter[search] (see users_v4.go / openapi Search)
    if (filters.search) params.append('search', filters.search)
    if (filters.username) params.append('filter[username]', filters.username)
    if (filters.email) params.append('filter[email]', filters.email)
    // Admin vs non-admin maps to filter[user_group]=admin|user (not filter[is_admin])
    if (filters.is_admin === 'true') params.append('filter[user_group]', 'admin')
    if (filters.is_admin === 'false') params.append('filter[user_group]', 'user')
    if (filters.enabled) params.append('filter[enabled]', filters.enabled)
    return params.toString()
  }

  const loadUsers = async () => {
    if (!token) return
    setLoadingUsers(true)
    try {
      const query = buildUsersQuery()
      const res = await fetch(`${apiBase}/users${query ? `?${query}` : ''}`, {
        headers: authHeader,
      })
      if (!res.ok) {
        const message = await parseError(res)
        toast.error(message)
        return
      }
      const payload = await res.json()
      setUsers(payload?.data?.items || [])
    } catch (err) {
      toast.error(`Failed to load users: ${(err as Error).message}`)
    } finally {
      setLoadingUsers(false)
    }
  }

  const resetUserForm = () => {
    setEditingUser(null)
    setCreateModalOpen(false)
    setUserForm({
      username: '',
      name: '',
      email: '',
      password: '',
      user_group: 'user',
      enabled: true,
    })
  }

  const handleUserFormChange = (field, value) => {
    setUserForm((prev) => ({ ...prev, [field]: value }))
  }

  const startEditUser = (user) => {
    setEditingUser(user)
    setUserForm({
      username: user.username || '',
      name: user.name || '',
      email: user.email || '',
      password: '',
      user_group: user.user_group || 'user',
      enabled: user.enabled !== false,
    })
  }

  const createUser = async () => {
    if (!usersPerms.add) {
      toast.error('Access denied')
      return
    }
    if (!token) return
    if (!userForm.username || !userForm.email || !userForm.password) {
      toast.error('Username, email, and password are required')
      return
    }
    setSavingUser(true)
    try {
      const res = await fetch(`${apiBase}/users`, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: userForm.username,
          name: String(userForm.name || '').trim(),
          email: userForm.email,
          password: userForm.password,
          user_group: userForm.user_group,
          enabled: userForm.enabled,
        }),
      })
      if (!res.ok) {
        const message = await parseError(res)
        toast.error(message)
        return
      }
      toast.success('User created')
      resetUserForm()
      await loadUsers()
    } catch (err) {
      toast.error(`Failed to create user: ${(err as Error).message}`)
    } finally {
      setSavingUser(false)
    }
  }

  const updateUser = async () => {
    if (!usersPerms.edit) {
      toast.error('Access denied')
      return
    }
    if (!token || !editingUser?.guid) return
    setSavingUser(true)
    try {
      const payload: any = {}
      if (userForm.email) payload.email = userForm.email
      if (userForm.password) payload.password = userForm.password
      if (userForm.user_group) payload.user_group = userForm.user_group
      payload.enabled = userForm.enabled
      payload.name = String(userForm.name || '').trim()

      const res = await fetch(`${apiBase}/users/${editingUser.guid}`, {
        method: 'PATCH',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const message = await parseError(res)
        toast.error(message)
        return
      }
      toast.success('User updated')
      resetUserForm()
      await loadUsers()
    } catch (err) {
      toast.error(`Failed to update user: ${(err as Error).message}`)
    } finally {
      setSavingUser(false)
    }
  }

  const deleteUser = async (user) => {
    if (!usersPerms.delete) {
      toast.error('Access denied')
      return
    }
    if (!token || !user?.guid) return
    const confirmed = await confirm({ message: `Delete user ${user.username || user.guid}?`, variant: 'destructive' })
    if (!confirmed) return
    setDeletingUser(user.guid)
    try {
      const res = await fetch(`${apiBase}/users/${user.guid}`, {
        method: 'DELETE',
        headers: authHeader,
      })
      if (!res.ok) {
        const message = await parseError(res)
        toast.error(message)
        return
      }
      toast.success('User deleted')
      await loadUsers()
    } catch (err) {
      toast.error(`Failed to delete user: ${(err as Error).message}`)
    } finally {
      setDeletingUser('')
    }
  }

  useEffect(() => {
    const onUsers =
      !!token && settingsOpen && activeSection === 'users'
    if (!onUsers) {
      usersSettingsActiveRef.current = false
      return
    }
    const justEntered = !usersSettingsActiveRef.current
    usersSettingsActiveRef.current = true
    if (justEntered) {
      void loadUsers()
      return
    }
    const id = window.setTimeout(() => {
      void loadUsers()
    }, 400)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- loadUsers uses latest filters via closure
  }, [token, settingsOpen, activeSection, filters])

  useEffect(() => {
    if (token) {
      loadMe()
    }
  }, [token])

  // Redirect to login when any API returns 401 (expired/invalid token)
  useEffect(() => {
    const onUnauth = () => setToken('')
    window.addEventListener('auth:unauthorized', onUnauth)
    return () => window.removeEventListener('auth:unauthorized', onUnauth)
  }, [])

  // Handle OAuth2 callback token
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const oauthToken = params.get('token')
    if (oauthToken) {
      setToken(oauthToken)
      toast.success('Logged in via OAuth2')
      window.history.replaceState({}, '', window.location.pathname + window.location.hash)
    }
  }, [])

  const renderSettingsPanel = () => {
    if (!canViewSection(role, activeSection)) {
      return <p className="muted">Access denied for this section.</p>
    }
    const perms = getSectionPerms(role, activeSection)
    const readOnly = !canWriteSection(role, activeSection)

    switch (activeSection) {
      case 'about':
        return <AboutPanel me={me} loading={loadingMe} onRefresh={loadMe} />
      case 'profile':
        return <ProfilePanel me={me} loading={loadingMe} onRefresh={loadMe} readOnly={readOnly} />
      case 'users':
        return (
          <UsersPanel
            filters={filters}
            setFilters={setFilters}
            users={users}
            loadingUsers={loadingUsers}
            onLoadUsers={loadUsers}
            userForm={userForm}
            onUserFormChange={handleUserFormChange}
            onCreate={createUser}
            onUpdate={updateUser}
            onDelete={deleteUser}
            onStartEdit={startEditUser}
            onResetForm={resetUserForm}
            editingUser={editingUser}
            savingUser={savingUser}
            deletingUser={deletingUser}
            createModalOpen={createModalOpen}
            onOpenCreateModal={() => setCreateModalOpen(true)}
            onCloseCreateModal={() => setCreateModalOpen(false)}
            canCreate={!!perms.add}
            canEdit={!!perms.edit}
            canDelete={!!perms.delete}
          />
        )
      case 'user-settings':
        return <UserSettingsPanel />
      case 'aliases':
        return <AliasesPanel readOnly={readOnly} />
      case 'advanced':
        return <AdvancedPanel readOnly={readOnly} />
      case 'mappings':
        return <MappingsPanel readOnly={readOnly} />
      case 'hepsubs':
        return <HepsubsPanel readOnly={readOnly} />
      case 'auth-tokens':
        return <AuthTokensPanel readOnly={readOnly} />
      case 'agent-subs':
        return <AgentSubsPanel readOnly={readOnly} />
      case 'dashboards':
        return <DashboardsPanel readOnly={readOnly} />
      case 'system':
        return <SystemPanel />
      case 'scripts':
        return <ScriptsPanel readOnly={readOnly} />
      case 'reset':
        return <ResetPanel readOnly={readOnly} />
      case 'api-docs':
        return <ApiDocsPanel />
      default:
        return <p className="muted">Select a section from the sidebar.</p>
    }
  }

  return (
    <ThemeProvider defaultTheme="dark" storageKey="vite-ui-theme">
      <div className="app">
        <main className="content">
          {!token ? (
            <LoginPage
              apiBase={apiBase}
              passwordAuthMethods={passwordAuthMethods}
              oauthProviderNames={oauthProviderNames}
              autoOAuthProvider={autoOAuthProvider}
              onLogin={setToken}
              onOAuth2={loginOAuth2}
            />
          ) : (
            <>
              {!settingsOpen ? (
                <DashboardPanel
                  apiBase={apiBase}
                  token={token}
                  me={me}
                  onOpenSettings={openSettings}
                  onOpenDashboard={openDashboard}
                  onLogout={logout}
                />
              ) : (
                <div className="flex min-h-screen flex-col bg-background">
                  <DashboardHeader
                    timeFrom=""
                    timeTo=""
                    onTimeChange={() => { }}
                    activePreset={null}
                    calendarPreset={null}
                    timeZone=""
                    onTimeZoneChange={() => { }}
                    userLabel={me?.username || 'User'}
                    onOpenSettings={openSettings}
                    onOpenDashboard={openDashboard}
                    onLogout={logout}
                    showTimeControls={false}
                    showSettings
                    showLogout
                    onBack={openDashboard}
                  />
                  <div className="flex min-h-0 flex-1 overflow-hidden">
                    <SettingsSidebar
                      activeSection={activeSection}
                      onSelect={setActiveSection}
                      role={role}
                    />
                    <main className="flex-1 overflow-y-auto bg-muted/20 p-6">
                      {renderSettingsPanel()}
                    </main>
                  </div>
                </div>
              )}
            </>
          )}

        </main>
        <WindowDock />
      </div>
    </ThemeProvider>
  )
}

export default App
