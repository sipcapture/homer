import { useMemo } from 'react'
import IFramePanel from './IFramePanel'

export default function GrafanaPanel({ config, onConfigChange }) {
  const normalized = useMemo(() => {
    if (config?.url) return config
    return { ...config, url: config?.grafanaUrl || '' }
  }, [config])

  return <IFramePanel config={normalized} onConfigChange={onConfigChange} />
}
