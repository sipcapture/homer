import { useEffect, useMemo, useRef, useState, type FormEvent } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { Separator } from "@/components/ui/separator"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { PasswordAuthMethodRow } from "./loginProviders"

/** Outlined input + label that floats up on focus or when non-empty. */
function FloatingAuthField({
  id,
  label,
  type = "text",
  value,
  onChange,
  autoComplete,
  disabled,
  autoFocus,
}: {
  id: string
  label: string
  type?: string
  value: string
  onChange: (v: string) => void
  autoComplete?: string
  disabled?: boolean
  autoFocus?: boolean
}) {
  const [focused, setFocused] = useState(false)
  const floated = focused || value.length > 0

  return (
    <div className="group relative">
      <label
        htmlFor={id}
        className={cn(
          "pointer-events-none absolute left-3 z-[1] select-none transition-all duration-200 ease-out",
          floated
            ? "top-2 text-[10px] font-medium uppercase tracking-wider text-primary"
            : "top-1/2 -translate-y-1/2 text-sm text-muted-foreground",
        )}
      >
        {label}
      </label>
      <Input
        id={id}
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        autoComplete={autoComplete}
        disabled={disabled}
        autoFocus={autoFocus}
        className={cn(
          "h-11 rounded-md border-border/70 bg-muted/30 px-3 pb-2 pt-5 text-sm shadow-sm",
          "transition-[border-color,box-shadow,background-color] duration-200",
          "focus-visible:border-primary/50 focus-visible:bg-background focus-visible:ring-2 focus-visible:ring-primary/15",
          "disabled:opacity-60",
        )}
      />
    </div>
  )
}

interface LoginPageProps {
  apiBase: string
  /** Password backends (internal DB, LDAP) from GET /auth/providers. */
  passwordAuthMethods: PasswordAuthMethodRow[]
  /** Enabled OAuth2 provider keys for redirect. */
  oauthProviderNames: string[]
  /** If set, immediately redirect to OAuth (homer-app auto_redirect). */
  autoOAuthProvider: string | null
  onLogin: (token: string) => void
  onOAuth2: (provider: string) => void
}

export function LoginPage({
  apiBase,
  passwordAuthMethods,
  oauthProviderNames,
  autoOAuthProvider,
  onLogin,
  onOAuth2,
}: LoginPageProps) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  const enabledPwd = useMemo(
    () => passwordAuthMethods.filter((m) => m.enable),
    [passwordAuthMethods],
  )

  const [authType, setAuthType] = useState("internal")

  useEffect(() => {
    if (!enabledPwd.length) return
    if (!enabledPwd.some((m) => m.type === authType)) {
      setAuthType(enabledPwd[0].type)
    }
  }, [enabledPwd, authType])

  const oauthRedirected = useRef(false)
  useEffect(() => {
    if (!autoOAuthProvider || oauthRedirected.current) return
    oauthRedirected.current = true
    onOAuth2(autoOAuthProvider)
  }, [autoOAuthProvider, onOAuth2])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError("")

    if (!username || !password) {
      setError("Login and password are required")
      return
    }

    const effectiveType =
      enabledPwd.length === 1 ? enabledPwd[0].type : authType || "internal"

    setLoading(true)
    try {
      const res = await fetch(`${apiBase}/auth/sessions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, type: effectiveType }),
      })

      if (!res.ok) {
        let detail = `Authentication failed (${res.status})`
        try {
          const payload = await res.json()
          if (payload?.error?.detail) detail = payload.error.detail
          else if (payload?.error?.title) detail = payload.error.title
        } catch { /* ignore */ }
        setError(detail)
        return
      }

      const payload = await res.json()
      const token = payload?.data?.token || ""
      if (!token) {
        setError("Token missing in server response")
        return
      }
      onLogin(token)
    } catch (err) {
      setError(`Connection failed: ${(err as Error).message}`)
    } finally {
      setLoading(false)
    }
  }

  const showAuthTypeSelect = enabledPwd.length > 1
  const showPasswordForm = enabledPwd.length > 0
  const oauthOnly = !showPasswordForm && oauthProviderNames.length > 0

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-background px-4">
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.03]"
        style={{
          backgroundImage:
            "linear-gradient(to right, currentColor 1px, transparent 1px), linear-gradient(to bottom, currentColor 1px, transparent 1px)",
          backgroundSize: "48px 48px",
        }}
      />

      <div className="pointer-events-none absolute left-1/2 top-1/3 -translate-x-1/2 -translate-y-1/2 h-[480px] w-[480px] rounded-full bg-primary/5 blur-[120px]" />

      <div className="relative z-10 flex w-full max-w-sm flex-col items-center gap-8">
        <div className="flex flex-col items-center gap-3">
          <img
            src="/logo.png"
            alt="Homer"
            className="h-14 w-14 drop-shadow-md"
          />
          <div className="text-center">
            <h1 className="font-heading text-xl font-semibold tracking-tight">
              Homer
            </h1>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Sign in to continue
            </p>
          </div>
        </div>

        <Card className="w-full overflow-hidden rounded-lg border-border/40 bg-card/85 shadow-xl shadow-black/10 ring-1 ring-border/30 backdrop-blur-md dark:bg-card/70 dark:shadow-black/30">
          <CardHeader className="space-y-1 border-b border-border/40 pb-4">
            <CardTitle className="font-heading text-lg tracking-tight">Sign in</CardTitle>
            <p className="text-[11px] text-muted-foreground">
              {oauthOnly ? "Continue with your identity provider" : "Use your deployment credentials"}
            </p>
          </CardHeader>
          <CardContent className="pt-5">
            {error && (
              <Alert variant="destructive" className="mb-5 rounded-md">
                <AlertDescription className="text-xs">{error}</AlertDescription>
              </Alert>
            )}

            {showPasswordForm && (
            <form onSubmit={handleSubmit} className="flex flex-col gap-5">
              {showAuthTypeSelect && (
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="login-auth-type" className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                    Authentication
                  </Label>
                  <Select value={authType} onValueChange={setAuthType} disabled={loading}>
                    <SelectTrigger id="login-auth-type" className="h-10 w-full rounded-md border-border/70 bg-muted/30">
                      <SelectValue placeholder="Method" />
                    </SelectTrigger>
                    <SelectContent>
                      {enabledPwd.map((m) => (
                        <SelectItem key={m.type} value={m.type}>
                          {m.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              <FloatingAuthField
                id="login-username"
                label="Login"
                value={username}
                onChange={setUsername}
                autoComplete="username"
                autoFocus
                disabled={loading}
              />

              <FloatingAuthField
                id="login-password"
                label="Password"
                type="password"
                value={password}
                onChange={setPassword}
                autoComplete="current-password"
                disabled={loading}
              />

              <Button
                type="submit"
                disabled={loading}
                className="mt-1 h-10 w-full rounded-md text-sm font-medium shadow-sm"
                size="lg"
              >
                {loading ? (
                  <span className="flex items-center gap-2">
                    <svg className="h-3.5 w-3.5 animate-spin" viewBox="0 0 24 24" fill="none">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Signing in...
                  </span>
                ) : (
                  "Sign in"
                )}
              </Button>
            </form>
            )}

            {oauthProviderNames.length > 0 && (
              <>
                {showPasswordForm && (
                <div className="relative my-5">
                  <Separator />
                  <span className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-card px-3 text-[10px] uppercase tracking-widest text-muted-foreground">
                    or
                  </span>
                </div>
                )}

                {oauthOnly && autoOAuthProvider && (
                  <p className="mb-3 text-center text-xs text-muted-foreground">
                    Redirecting to sign in...
                  </p>
                )}

                <div className="flex flex-col gap-2">
                  {oauthProviderNames.map((provider) => (
                    <Button
                      key={provider}
                      variant="outline"
                      className="w-full capitalize"
                      onClick={() => onOAuth2(provider)}
                      disabled={loading}
                    >
                      Continue with {provider}
                    </Button>
                  ))}
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <p className="text-[10px] tracking-wide text-muted-foreground/50">
          Homer v11
        </p>
      </div>
    </div>
  )
}
