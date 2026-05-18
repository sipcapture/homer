import DashboardLayout from '../dashboard/DashboardLayout'

export default function DashboardPanel({
  apiBase,
  token,
  me,
  onOpenSettings,
  onOpenDashboard,
  onLogout,
  onOpenResetSettings,
}: {
  apiBase: string
  token: string
  me: { username?: string } | null
  onOpenSettings: () => void
  onOpenDashboard: () => void
  onLogout: () => void
  onOpenResetSettings?: () => void
}) {
  return (
    <DashboardLayout
      apiBase={apiBase}
      token={token}
      me={me}
      onOpenSettings={onOpenSettings}
      onOpenDashboard={onOpenDashboard}
      onLogout={onLogout}
      onOpenResetSettings={onOpenResetSettings}
    />
  )
}
