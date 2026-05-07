import { useState, useRef } from 'react'
import { Upload } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { useDashboard } from '../context/DashboardContext'

export default function PcapUploaderPanel() {
  const { apiBase, authHeader } = useDashboard()
  const [status, setStatus] = useState('')
  const [statusType, setStatusType] = useState<'info' | 'success' | 'error'>('info')
  const [uploading, setUploading] = useState(false)
  const [dragOver, setDragOver] = useState(false)
  const [overrideToCurrentTime, setOverrideToCurrentTime] = useState(false)
  /** empty = auto by SIP method; otherwise force DuckLake SIP table */
  const [forceSipTable, setForceSipTable] = useState<'auto' | 'call' | 'registration' | 'default'>('auto')
  const inputRef = useRef<HTMLInputElement | null>(null)

  const upload = async (file: File | undefined) => {
    if (!file) return
    setUploading(true)
    setStatus('')
    try {
      const formData = new FormData()
      formData.append('file', file)
      if (overrideToCurrentTime) {
        formData.append('override_to_current_time', 'true')
      }
      if (forceSipTable !== 'auto') {
        formData.append('force_sip_table', forceSipTable)
      }
      const res = await fetch(`${apiBase}/imports/pcap`, {
        method: 'POST',
        headers: { Authorization: authHeader.Authorization },
        body: formData,
      })
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error(err?.error?.detail || `Upload failed (${res.status})`)
      }
      const data = await res.json()
      const inserted = data?.data?.inserted ?? 0
      const rejected = data?.data?.rejected ?? 0
      setStatus(
        `Uploaded ${file.name}: ${inserted} SIP message(s) inserted` +
          (rejected ? `, ${rejected} packet(s) skipped` : '') +
          (overrideToCurrentTime ? ' (timestamps shifted to now)' : '') +
          (forceSipTable !== 'auto' ? ` (forced table: ${forceSipTable})` : ''),
      )
      setStatusType('success')
    } catch (err) {
      setStatus(err.message)
      setStatusType('error')
    } finally {
      setUploading(false)
    }
  }

  const handleDrop = (e) => {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer?.files?.[0]
    if (file) upload(file)
  }

  return (
    <div className="flex h-full flex-col gap-2 p-2">
      <div className="flex flex-col gap-1.5 text-xs text-muted-foreground">
        <label className="flex items-center gap-2">
          <span className="shrink-0">SIP table</span>
          <select
            className="h-7 max-w-[220px] rounded-md border border-border bg-background px-2 text-xs text-foreground"
            value={forceSipTable}
            onChange={(e) =>
              setForceSipTable(e.target.value as 'auto' | 'call' | 'registration' | 'default')
            }
            disabled={uploading}
          >
            <option value="auto">Auto (by SIP method)</option>
            <option value="call">Force: call (hep_proto_1_call)</option>
            <option value="registration">Force: registration (hep_proto_1_registration)</option>
            <option value="default">Force: default (hep_proto_1_default)</option>
          </select>
        </label>
        <Label className="flex cursor-pointer items-center gap-2">
          <input
            type="checkbox"
            className="h-3.5 w-3.5 rounded border border-border"
            checked={overrideToCurrentTime}
            onChange={(e) => setOverrideToCurrentTime(e.target.checked)}
            disabled={uploading}
          />
          Override capture time to current time (preserve relative spacing)
        </Label>
      </div>
      <div
        className={cn(
          'flex flex-1 cursor-pointer flex-col items-center justify-center gap-2 border-2 border-dashed border-border bg-card/40 p-4 text-center text-xs text-muted-foreground transition-colors hover:border-primary/50 hover:bg-accent/40',
          dragOver && 'border-primary bg-accent/60 text-foreground',
        )}
        onDragOver={(e) => {
          e.preventDefault()
          setDragOver(true)
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
      >
        <Upload className="h-8 w-8 opacity-60" />
        <span>{uploading ? 'Uploading...' : 'Drop PCAP file here or click to browse'}</span>
        <input
          ref={inputRef}
          type="file"
          accept=".pcap,.pcapng,.cap"
          className="hidden"
          onChange={(e) => upload(e.target.files?.[0])}
        />
      </div>
      {status && (
        <Alert variant={statusType === 'error' ? 'destructive' : 'default'} className="py-2">
          <AlertDescription className="text-xs">{status}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}
