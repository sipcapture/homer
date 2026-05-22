import DashboardLayout from '../dashboard/DashboardLayout'

export default function DashboardPanel({
  apiBase,
  token,
  me,
  userAvatarUrl,
  onOpenSettings,
  onOpenDashboard,
  onLogout,
  onOpenResetSettings,
}: {
  apiBase: string
  token: string
  me: { username?: string; display_name?: string } | null
  userAvatarUrl?: string | null
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
      userAvatarUrl={userAvatarUrl}
      onOpenSettings={onOpenSettings}
      onOpenDashboard={onOpenDashboard}
      onLogout={onLogout}
      onOpenResetSettings={onOpenResetSettings}
    />
  )
}
