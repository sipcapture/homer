import { RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { SettingsPageHeader } from './SettingsPageHeader'
import UserAvatar from '@/components/UserAvatar'

interface Me {
  username?: string
  display_name?: string
  guid?: string
  group?: string
  admin?: boolean
}

interface AboutPanelProps {
  me: Me | null
  avatarUrl?: string | null
  loading?: boolean
  onRefresh?: () => void
}

export default function AboutPanel({ me, avatarUrl = null, loading, onRefresh }: AboutPanelProps) {
  return (
    <div>
      <SettingsPageHeader
        title="About"
        description="Your current profile and account details."
        actions={
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={loading}>
            <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
            Refresh
          </Button>
        }
      />
      <Card>
        <CardHeader>
          <CardTitle>Account</CardTitle>
          <CardDescription>Read-only identity from your current session.</CardDescription>
        </CardHeader>
        <CardContent>
          {me ? (
            <>
            <div className="mb-4 flex items-center gap-3">
              <UserAvatar
                label={me.display_name || me.username || 'User'}
                imageUrl={avatarUrl}
                size="md"
              />
              <div>
                <p className="text-sm font-medium text-foreground">{me.display_name || me.username}</p>
                <p className="text-xs text-muted-foreground">{me.username}</p>
              </div>
            </div>
            <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Username" value={me.username || '—'} />
              <Field label="Name" value={me.display_name || '—'} />
              <Field label="GUID" value={me.guid || '—'} mono />
              <Field label="Group" value={me.group || '—'} />
              <div className="flex flex-col gap-1">
                <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Admin
                </dt>
                <dd>
                  <Badge variant={me.admin ? 'default' : 'secondary'}>
                    {me.admin ? 'Yes' : 'No'}
                  </Badge>
                </dd>
              </div>
            </dl>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">No profile loaded.</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Field({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </dt>
      <dd className={cn('text-sm text-foreground', mono && 'font-mono')}>{value}</dd>
    </div>
  )
}
