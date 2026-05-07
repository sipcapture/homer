import { useState, useRef, useEffect } from 'react'
import { Download, X } from 'lucide-react'
import { apiPost, apiGet, apiDownloadUrl, handleUnauthorized } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CrudModal, Field } from './CrudTable'

interface ExportModalProps {
  searchPayload: Record<string, unknown>
  onClose: () => void
}

export default function ExportModal({ searchPayload, onClose }: ExportModalProps) {
  const [format, setFormat] = useState('pcap')
  const [status, setStatus] = useState('')
  const [, setExportId] = useState('')
  const [exporting, setExporting] = useState(false)
  const [downloadUrl, setDownloadUrl] = useState('')
  const [error, setError] = useState('')
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  const startExport = async () => {
    setExporting(true)
    setError('')
    setStatus('Starting export...')
    setDownloadUrl('')
    try {
      const data: any = await apiPost('/exports', { type: format, query: searchPayload })
      const id = data?.data?.export_id
      if (!id) {
        setError('No export ID returned')
        setExporting(false)
        return
      }
      setExportId(id)
      setStatus('Export started. Checking status...')
      pollStatus(id)
    } catch (err) {
      setError(err.message)
      setExporting(false)
    }
  }

  const pollStatus = (id: string) => {
    let attempts = 0
    pollingRef.current = setInterval(async () => {
      attempts++
      try {
        const data = await apiGet(`/exports/${id}`)
        const s = data?.data?.status
        if (s === 'completed' || s === 'ready') {
          if (pollingRef.current) clearInterval(pollingRef.current)
          pollingRef.current = null
          setStatus('Export ready for download')
          setDownloadUrl(
            data?.data?.download_url || apiDownloadUrl(`/exports/${id}/download`),
          )
          setExporting(false)
        } else if (s === 'failed' || s === 'error') {
          if (pollingRef.current) clearInterval(pollingRef.current)
          pollingRef.current = null
          setError('Export failed')
          setExporting(false)
        } else {
          setStatus(`Exporting... (${s || 'in progress'})`)
        }
      } catch (err) {
        if (attempts > 30) {
          if (pollingRef.current) clearInterval(pollingRef.current)
          pollingRef.current = null
          setError(`Export polling failed: ${err.message}`)
          setExporting(false)
        }
      }
    }, 2000)
  }

  const download = () => {
    if (!downloadUrl) return
    const link = document.createElement('a')
    link.href = downloadUrl
    link.download = `export.${format}`
    const token = localStorage.getItem('homer_v4_token')
    if (token && downloadUrl.startsWith('/')) {
      fetch(downloadUrl, { headers: { Authorization: `Bearer ${token}` } })
        .then((r) => {
          if (r.status === 401) {
            handleUnauthorized()
            throw new Error('Unauthorized')
          }
          return r.blob()
        })
        .then((blob) => {
          const url = URL.createObjectURL(blob)
          link.href = url
          link.click()
          URL.revokeObjectURL(url)
        })
        .catch(() => {
          link.click()
        })
    } else {
      link.click()
    }
  }

  return (
    <CrudModal title="Export Data" open onClose={onClose}>
      <div className="space-y-4">
        <Field label="Export Format">
          <Select value={format} onValueChange={setFormat}>
            <SelectTrigger className="w-full" disabled={exporting}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="pcap">PCAP</SelectItem>
              <SelectItem value="text">Text</SelectItem>
            </SelectContent>
          </Select>
        </Field>

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

        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            <X className="mr-1.5 size-4" />
            Close
          </Button>
          {downloadUrl ? (
            <Button onClick={download}>
              <Download className="mr-1.5 size-4" />
              Download
            </Button>
          ) : (
            <Button onClick={startExport} disabled={exporting}>
              <Download className="mr-1.5 size-4" />
              {exporting ? 'Exporting...' : 'Start Export'}
            </Button>
          )}
        </div>
      </div>
    </CrudModal>
  )
}
