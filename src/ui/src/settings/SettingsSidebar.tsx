import {
  Activity,
  BookOpen,
  Code,
  Info,
  KeyRound,
  LayoutDashboard,
  type LucideIcon,
  Map,
  Network,
  Radio,
  RotateCcw,
  Server,
  Sliders,
  User,
  UserCog,
  Users,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { canViewSection } from './permissions'

type SidebarItem = { key: string; label: string; icon: LucideIcon }

const sections: { group: string; items: SidebarItem[] }[] = [
  {
    group: 'General',
    items: [
      { key: 'profile', label: 'Profile', icon: User },
      { key: 'about', label: 'About', icon: Info },
      { key: 'users', label: 'Users', icon: Users },
      { key: 'user-settings', label: 'User Settings', icon: UserCog },
    ],
  },
  {
    group: 'Configuration',
    items: [
      { key: 'aliases', label: 'IP Aliases', icon: Network },
      { key: 'advanced', label: 'Advanced', icon: Sliders },
      { key: 'mappings', label: 'Mappings', icon: Map },
      { key: 'hepsubs', label: 'HEP Subscriptions', icon: Radio },
      { key: 'auth-tokens', label: 'Auth Tokens', icon: KeyRound },
      { key: 'agent-subs', label: 'Agent Subscriptions', icon: Activity },
      { key: 'dashboards', label: 'Dashboards', icon: LayoutDashboard },
      { key: 'scripts', label: 'Scripts', icon: Code },
    ],
  },
  {
    group: 'System',
    items: [
      { key: 'system', label: 'System Overview', icon: Server },
      { key: 'reset', label: 'Reset', icon: RotateCcw },
      { key: 'api-docs', label: 'API Documentation', icon: BookOpen },
    ],
  },
]

interface SettingsSidebarProps {
  activeSection: string
  onSelect: (key: string) => void
  role?: string
}

export function SettingsSidebar({
  activeSection,
  onSelect,
  role,
}: SettingsSidebarProps) {
  const filtered = sections
    .map((g) => ({
      ...g,
      items: g.items.filter((i) => canViewSection(role, i.key)),
    }))
    .filter((g) => g.items.length > 0)

  return (
    <aside className="flex min-h-0 w-64 shrink-0 flex-col border-r border-border bg-card">
      <nav className="flex-1 overflow-y-auto p-3">
        {filtered.map((group) => (
          <div key={group.group} className="mb-4 last:mb-0">
            <div className="mb-1 px-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              {group.group}
            </div>
            <div className="flex flex-col gap-0.5">
              {group.items.map((item) => {
                const Icon = item.icon
                const active = activeSection === item.key
                return (
                  <button
                    key={item.key}
                    type="button"
                    onClick={() => onSelect(item.key)}
                    className={cn(
                      'flex w-full items-center gap-2.5 rounded-none px-2.5 py-1.5 text-sm font-medium transition-colors',
                      active
                        ? 'bg-primary text-primary-foreground'
                        : 'text-foreground/80 hover:bg-accent hover:text-accent-foreground',
                    )}
                  >
                    <Icon className="size-4 shrink-0" />
                    <span className="truncate">{item.label}</span>
                  </button>
                )
              })}
            </div>
          </div>
        ))}
      </nav>
    </aside>
  )
}
