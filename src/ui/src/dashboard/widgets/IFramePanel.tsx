import { useEffect, useMemo, useState } from 'react'
import { ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EditIconButton } from '@/components/ui/table-action-buttons'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { normalizeHttpUrl } from '@/lib/safeUrl'

export default function IFramePanel({ config, onConfigChange }) {
  const savedUrl = config?.url || ''
  const [editing, setEditing] = useState(!savedUrl)
  const [url, setUrl] = useState(savedUrl)

  useEffect(() => {
    if (!editing) setUrl(savedUrl)
  }, [savedUrl, editing])

  const src = useMemo(() => normalizeHttpUrl(savedUrl || url || ''), [savedUrl, url])

  if (editing) {
    return (
      <div className="flex h-full flex-col gap-3 p-3">
        <div className="grid gap-1.5">
          <Label htmlFor="iframe-url" className="text-xs text-muted-foreground">
            URL
          </Label>
          <Input
            id="iframe-url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://grafana.example/d/…"
            className="h-8 text-xs"
          />
          <p className="text-[10px] leading-snug text-muted-foreground">
            Many sites forbid embedding (X-Frame-Options / CSP). Grafana: set{' '}
            <code className="rounded bg-muted px-0.5">security.allow_embedding = true</code> and matching{' '}
            <code className="rounded bg-muted px-0.5">root_url</code>. If the frame stays blank, use “Open” below.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            onClick={() => {
              const next = normalizeHttpUrl(url)
              if (!next) return
              setUrl(next)
              onConfigChange?.({ ...config, url: next })
              setEditing(false)
            }}
          >
            Load
          </Button>
          <Button type="button" size="sm" variant="ghost" className="h-8 text-xs" onClick={() => setEditing(false)}>
            Cancel
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="relative h-full w-full">
      {src ? (
        <iframe
          key={src}
          src={src}
          title="Embedded content"
          className="h-full w-full border-none"
          referrerPolicy="strict-origin-when-cross-origin"
          allow="fullscreen; clipboard-read; clipboard-write; autoplay"
          /** No sandbox: restrictive sandbox breaks Grafana and other apps (forms, storage, redirects). */
        />
      ) : (
        <div className="flex h-full items-center justify-center p-4 text-center text-xs text-muted-foreground">
          No URL configured.
        </div>
      )}
      <div className="pointer-events-auto absolute right-2 top-2 z-20 flex items-center gap-1 rounded-md border border-border/60 bg-background/95 p-0.5 shadow-sm backdrop-blur-sm">
        {src ? (
          <Button
            type="button"
            size="icon-sm"
            variant="outline"
            className="h-8 w-8 shrink-0"
            title="Open in new tab"
            asChild
          >
            <a href={src} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="h-3.5 w-3.5" />
            </a>
          </Button>
        ) : null}
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 shrink-0 gap-1 px-2 text-xs"
          title="Change embedded URL"
          onClick={() => {
            setUrl(savedUrl)
            setEditing(true)
          }}
        >
          <span className="hidden sm:inline">Change URL</span>
          <span className="sm:hidden">URL</span>
        </Button>
        <EditIconButton
          aria-label="Edit URL"
          title="Edit URL"
          size="icon-sm"
          variant="outline"
          className="shrink-0"
          onClick={() => {
            setUrl(savedUrl)
            setEditing(true)
          }}
        />
      </div>
    </div>
  )
}
