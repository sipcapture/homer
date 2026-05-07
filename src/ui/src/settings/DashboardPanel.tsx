import DashboardLayout from '../dashboard/DashboardLayout'

export default function DashboardPanel({
  apiBase,
  token,
  me,
  onOpenSettings,
  onOpenDashboard,
  onLogout,
}) {
  return (
    <DashboardLayout
      apiBase={apiBase}
      token={token}
      me={me}
      onOpenSettings={onOpenSettings}
      onOpenDashboard={onOpenDashboard}
      onLogout={onLogout}
    />
  )
}
