export const ROLE = {
  ADMIN: 'admin',
  COMMON: 'commonUser',
  EXTERNAL: 'external',
}

const ACCESS = {
  [ROLE.ADMIN]: {
    profile: { view: true, edit: true },
    about: { view: true },
    users: { view: true, add: true, edit: true, delete: true },
    'user-settings': { view: true, add: true, edit: true, delete: true },
    aliases: { view: true, add: true, edit: true, delete: true },
    advanced: { view: true, add: true, edit: true, delete: true },
    mappings: { view: true, add: true, edit: true, delete: true, reset: true },
    hepsubs: { view: true, add: true, edit: true, delete: true },
    'auth-tokens': { view: true, add: true, edit: true, delete: true },
    'agent-subs': { view: true, delete: true },
    dashboards: { view: true, add: true, edit: true, delete: true },
    scripts: { view: true, add: true, edit: true, delete: true },
    system: { view: true },
    reset: { view: true, serverReset: true },
    'api-docs': { view: true },
  },
  [ROLE.COMMON]: {
    profile: { view: true, edit: true },
    about: { view: true },
    // Users management (listing, creating, editing other accounts) is
    // admin-only — the backend also 403s /api/v4/users for non-admins.
    // Common users edit their own profile via the Profile tab, which
    // hits /api/v4/me and does not require the Users section.
    'user-settings': { view: true, add: true, edit: true, delete: true },
    // Dashboards are per-owner on the server (V4DashboardsList scopes
    // by JWT username), so a common user managing their own saved
    // layouts is safe and never sees other users' dashboards.
    dashboards: { view: true, add: true, edit: true, delete: true },
    advanced: { view: true },
    reset: { view: true },
  },
  [ROLE.EXTERNAL]: {
    profile: { view: true, edit: true },
    advanced: { view: true },
    reset: { view: true },
  },
}

export function detectRole(me) {
  if (!me) return ROLE.COMMON
  if (me.admin === true || me.user_group === 'admin') return ROLE.ADMIN
  if (me.isExternal === true || me.external === true) return ROLE.EXTERNAL
  return ROLE.COMMON
}

export function getSectionPerms(role, sectionKey) {
  return ACCESS[role]?.[sectionKey] || {}
}

export function canViewSection(role, sectionKey) {
  return !!getSectionPerms(role, sectionKey).view
}

export function canWriteSection(role, sectionKey) {
  const p = getSectionPerms(role, sectionKey)
  return !!(p.add || p.edit || p.delete || p.reset)
}

/** Server-side reset (dashboards, global mappings) — admin only. */
export function canServerReset(role) {
  return !!getSectionPerms(role, 'reset').serverReset
}

