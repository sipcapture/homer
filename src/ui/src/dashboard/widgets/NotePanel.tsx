import { useState } from 'react'
import { Textarea } from '@/components/ui/textarea'

export default function NotePanel({ config, onConfigChange }) {
  const [text, setText] = useState(config?.text || '')

  const handleChange = (e) => {
    setText(e.target.value)
    onConfigChange?.({ ...config, text: e.target.value })
  }

  return (
    <div className="flex h-full flex-col p-2">
      <Textarea
        value={text}
        onChange={handleChange}
        placeholder="Add notes here..."
        className="h-full flex-1 resize-none text-xs"
      />
    </div>
  )
}
