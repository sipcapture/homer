import { useState, type FormEvent } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { apiPatch } from './api'

interface ForcePasswordChangeProps {
  onChanged: () => void
  onLogout: () => void
}

export function ForcePasswordChange({ onChanged, onLogout }: ForcePasswordChangeProps) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    const next = password.trim()
    if (!next) {
      setError('New password is required')
      return
    }
    if (next === 'sipcapture') {
      setError('Choose a password other than the historical default')
      return
    }
    if (next !== confirm) {
      setError('Passwords do not match')
      return
    }
    setSaving(true)
    try {
      await apiPatch('/me', { password: next })
      onChanged()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-background px-4">
      <Card className="relative z-10 w-full max-w-md border-border/70 shadow-lg">
        <CardHeader>
          <CardTitle>Change default password</CardTitle>
          <CardDescription>
            You signed in with the historical default password. Choose a new password, then sign in again.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit}>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <Input
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="New password"
              disabled={saving}
              autoFocus
            />
            <Input
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder="Confirm password"
              disabled={saving}
            />
            <div className="flex items-center justify-between gap-2">
              <Button type="button" variant="ghost" onClick={onLogout} disabled={saving}>
                Sign out
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? 'Saving…' : 'Save and sign in again'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
