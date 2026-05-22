import { useState } from 'react'
import { RefreshCw, Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { SettingsPageHeader } from './SettingsPageHeader'
import UserAvatar from '@/components/UserAvatar'
import { apiPatch } from '../api'

interface Me {
  username?: string
  display_name?: string
  guid?: string
  group?: string
  user_group?: string
  admin?: boolean
  email?: string
}

interface ProfilePanelProps {
  me: Me | null
  avatarUrl?: string | null
  loading?: boolean
  onRefresh?: () => void
  readOnly?: boolean
}

export default function ProfilePanel({
  me,
  avatarUrl = null,
  loading,
  onRefresh,
  readOnly = false,
}: ProfilePanelProps) {
  const [email, setEmail] = useState(me?.email || '')
  const [password, setPassword] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const saveProfile = async () => {
    if (readOnly) {
      setError('Access denied')
      return
    }
    setSaving(true)
    setStatus('')
    setError('')
    try {
      const payload: Record<string, string> = {}
      if (email.trim()) payload.email = email.trim()
      if (password.trim()) payload.password = password
      if (!Object.keys(payload).length) {
        setError('Nothing to update')
        return
      }
      await apiPatch('/me', payload)
      setPassword('')
      setStatus('Profile updated')
      onRefresh?.()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Profile"
        description="View your identity and update your email or password."
        actions={
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={loading}>
            <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
            Refresh
          </Button>
        }
      />

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {status && (
        <Alert>
          <AlertDescription>{status}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader className="flex flex-row items-center gap-4 space-y-0">
          <UserAvatar
            label={me?.display_name || me?.username || 'User'}
            imageUrl={avatarUrl}
            size="md"
          />
          <div className="min-w-0 flex-1">
          <CardTitle>Identity</CardTitle>
          <CardDescription>
            Details inherited from your authentication provider.
          </CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Username" value={me?.username || '—'} />
            <Field label="GUID" value={me?.guid || '—'} mono />
            <Field label="Group" value={me?.group || me?.user_group || '—'} />
            <div className="flex flex-col gap-1">
              <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Admin
              </dt>
              <dd>
                <Badge variant={me?.admin ? 'default' : 'secondary'}>
                  {me?.admin ? 'Yes' : 'No'}
                </Badge>
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Credentials</CardTitle>
          <CardDescription>
            Update your contact email or rotate your password.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-2">
            <Label htmlFor="profile-email">Email</Label>
            <Input
              id="profile-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="user@example.com"
              disabled={readOnly}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="profile-password">New password</Label>
            <Input
              id="profile-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Leave blank to keep current"
              disabled={readOnly}
            />
          </div>
          {readOnly && (
            <p className="text-sm text-muted-foreground">
              Profile editing is disabled for your role.
            </p>
          )}
        </CardContent>
        <CardFooter className="justify-end">
          <Button onClick={saveProfile} disabled={saving || readOnly}>
            <Save className="mr-1.5 size-4" />
            {saving ? 'Saving...' : 'Save profile'}
          </Button>
        </CardFooter>
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
