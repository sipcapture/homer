import { useState } from 'react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

export default function CodeEditorPanel({ config, onConfigChange }) {
  const [code, setCode] = useState(config?.code || '')
  const [lang, setLang] = useState(config?.lang || 'sip')

  const handleChange = (e) => {
    setCode(e.target.value)
    onConfigChange?.({ ...config, code: e.target.value, lang })
  }

  return (
    <div className="flex h-full flex-col gap-2 p-2">
      <div className="flex items-center gap-2">
        <Select
          value={lang}
          onValueChange={(v) => {
            setLang(v)
            onConfigChange?.({ ...config, code, lang: v })
          }}
        >
          <SelectTrigger className="h-7 w-32 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="sip">SIP</SelectItem>
            <SelectItem value="json">JSON</SelectItem>
            <SelectItem value="sql">SQL</SelectItem>
            <SelectItem value="text">Plain Text</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Textarea
        value={code}
        onChange={handleChange}
        placeholder={lang === 'sip' ? 'INVITE sip:...' : lang === 'json' ? '{ }' : 'Enter code...'}
        spellCheck={false}
        className="h-full flex-1 resize-none font-mono text-xs"
      />
    </div>
  )
}
