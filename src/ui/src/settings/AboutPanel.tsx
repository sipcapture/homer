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

const QXIP_URL = 'https://qxip.net/'
const QXIP_LOGO_URL = 'https://qxip.net/images/qxip.png'
const HEPIC_URL = 'https://hepic.tel/'
const HEPIC_LOGO_URL = 'https://hepic.tel/images/contents/hero-phone.png'

export default function AboutPanel({ me, avatarUrl = null, loading, onRefresh }: AboutPanelProps) {
  return (
    <div className="space-y-6">
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

      <Card>
        <CardHeader>
          <CardTitle>Sponsored by QXIP and Hepic</CardTitle>
          <CardDescription>
            HOMER is developed by QXIP; Hepic extends the HEP stack with carrier-grade RTC observability.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-8 sm:flex-row sm:items-start sm:gap-10">
          <a
            href={QXIP_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-w-0 flex-1 flex-col gap-3 rounded-lg outline-none ring-offset-background transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            <img
              src={QXIP_LOGO_URL}
              alt="QXIP — Telecom observability"
              className="h-auto max-h-16 w-full max-w-xs object-contain object-left"
              loading="lazy"
              decoding="async"
            />
            <span className="text-sm font-medium text-primary underline-offset-4 hover:underline">
              qxip.net
            </span>
          </a>
          <a
            href={HEPIC_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-w-0 flex-1 flex-col gap-3 rounded-lg outline-none ring-offset-background transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            <img
              src={HEPIC_LOGO_URL}
              alt="HEPIC — VoIP and RTC analyzer"
              className="h-auto max-h-48 w-full max-w-md object-contain object-left"
              loading="lazy"
              decoding="async"
            />
            <span className="text-sm font-medium text-primary underline-offset-4 hover:underline">
              hepic.tel
            </span>
          </a>
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
