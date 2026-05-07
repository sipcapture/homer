import { useMemo } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { StreamLanguage } from '@codemirror/language'
import { lua } from '@codemirror/legacy-modes/mode/lua'
import { EditorView } from '@codemirror/view'
import { useTheme } from '@/components/theme/theme-provider'
import { cn } from '@/lib/utils'

/** Ensures long scripts scroll inside the editor when height is bounded (fixes % height chain in flex dialogs). */
const scrollerOverflow = EditorView.theme({
  '.cm-scroller': {
    overflow: 'auto',
  },
})

type Props = {
  value: string
  onChange: (value: string) => void
  /** Pixel height when `height` is not set */
  minHeight?: number
  /** Passed to CodeMirror as `height` (e.g. `100%` inside a sized parent) */
  height?: string
  readOnly?: boolean
  className?: string
  placeholder?: string
}

export default function ScriptEditor({
  value,
  onChange,
  minHeight = 320,
  height,
  readOnly = false,
  className,
  placeholder,
}: Props) {
  const { theme } = useTheme()
  const resolvedTheme = useMemo<'dark' | 'light'>(() => {
    if (theme === 'dark') return 'dark'
    if (theme === 'light') return 'light'
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  }, [theme])

  const extensions = useMemo(
    () => [StreamLanguage.define(lua), scrollerOverflow],
    [],
  )

  return (
    <div
      className={cn(
        'flex min-h-0 flex-1 flex-col overflow-hidden rounded-md border border-input bg-background text-xs [&_.cm-editor]:min-h-0',
        className,
      )}
    >
      <CodeMirror
        value={value}
        theme={resolvedTheme}
        height={height ?? `${minHeight}px`}
        extensions={extensions}
        editable={!readOnly}
        readOnly={readOnly}
        placeholder={placeholder}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: !readOnly,
          foldGutter: true,
          autocompletion: true,
          bracketMatching: true,
          closeBrackets: true,
          indentOnInput: true,
        }}
        onChange={(v) => onChange(v)}
      />
    </div>
  )
}
