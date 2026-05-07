import type { Completion, CompletionContext, CompletionSource } from '@codemirror/autocomplete'

/** Nested: catalog → schema → table → column names */
export type LakeSqlSchema = Record<string, Record<string, Record<string, string[]>>>

function findKeyCI(obj: Record<string, unknown>, want: string): string | null {
  const lw = want.toLowerCase()
  for (const k of Object.keys(obj)) {
    if (k.toLowerCase() === lw) return k
  }
  return null
}

function lookupColumns(schema: LakeSqlSchema, parts: string[]): string[] | null {
  if (parts.length < 2) return null
  if (parts.length >= 3) {
    let cur: unknown = schema
    for (const p of parts) {
      if (typeof cur !== 'object' || cur === null) return null
      const k = findKeyCI(cur as Record<string, unknown>, p)
      if (!k) return null
      cur = (cur as Record<string, unknown>)[k]
    }
    return Array.isArray(cur) ? (cur as string[]) : null
  }
  const schemaName = parts[0]
  const tableName = parts[1]
  for (const cat of Object.keys(schema)) {
    const sch = schema[cat]
    if (!sch) continue
    const sk = findKeyCI(sch as unknown as Record<string, unknown>, schemaName)
    if (!sk) continue
    const tables = sch[sk]
    if (!tables) continue
    const tk = findKeyCI(tables as unknown as Record<string, unknown>, tableName)
    if (!tk) continue
    const cols = tables[tk]
    if (Array.isArray(cols) && cols.length) return cols
  }
  return null
}

function findLastMatchIndex(s: string, re: RegExp): number {
  let last = -1
  let m: RegExpExecArray | null
  const r = new RegExp(re.source, re.flags.includes('g') ? re.flags : `${re.flags}g`)
  while ((m = r.exec(s)) !== null) last = m.index
  return last
}

/** FROM … until WHERE|GROUP|ORDER|LIMIT|HAVING|set operators */
function sliceAfterFromKeyword(doc: string, fromIdx: number): string {
  const tail = doc.slice(fromIdx)
  const m = tail.match(/^from\s+/i)
  if (!m) return ''
  const start = fromIdx + m[0].length
  const rest = doc.slice(start)
  const endRel = rest.search(/\b(where|group|order|limit|having|union|intersect|except)\b/i)
  return endRel < 0 ? rest : rest.slice(0, endRel)
}

/** Table refs from one FROM…JOIN… segment (comma / JOIN). */
function splitFromTables(clause: string): string[] {
  const refs: string[] = []
  const joinSplit = clause.split(/\b(?:inner\s+|left\s+|right\s+|full\s+|cross\s+)?join\b/gi)
  for (let i = 0; i < joinSplit.length; i++) {
    let piece = joinSplit[i].trim()
    if (i > 0) piece = piece.replace(/\bon\b[\s\S]*$/i, '').trim()
    for (const commaPart of piece.split(/\s*,\s*/)) {
      const token = commaPart.trim().split(/\s+/)[0]
      if (token && !/^(on|using)$/i.test(token) && !token.startsWith('(')) refs.push(token)
    }
  }
  return refs
}

function pathFromTableToken(token: string): string[] | null {
  const ref = token.split(/\s+/)[0]
  if (!ref || !ref.includes('.')) return null
  const parts = ref.split('.').filter(Boolean)
  return parts.length >= 2 ? parts : null
}

function collectTablePathsFromDocUpTo(doc: string, pos: number): string[][] {
  const head = doc.slice(0, pos)
  const seen = new Set<string>()
  const out: string[][] = []
  const re = /\bfrom\s+/gi
  let m: RegExpExecArray | null
  while ((m = re.exec(head)) !== null) {
    const clause = sliceAfterFromKeyword(doc, m.index)
    for (const t of splitFromTables(clause)) {
      const p = pathFromTableToken(t)
      if (!p) continue
      const key = p.join('.')
      if (seen.has(key)) continue
      seen.add(key)
      out.push(p)
    }
  }
  return out
}

/** SELECT-list: after last SELECT, before the matching FROM (same slice as head). */
function collectTablePathsForSelectList(doc: string, pos: number): string[][] {
  const head = doc.slice(0, pos)
  if (findLastMatchIndex(head, /\bselect\b/gi) < 0) return []
  const tail = doc.slice(pos)
  const fromRel = tail.search(/\bfrom\b/i)
  if (fromRel < 0) return []
  const fromAbs = pos + fromRel
  const clause = sliceAfterFromKeyword(doc, fromAbs)
  const seen = new Set<string>()
  const out: string[][] = []
  for (const t of splitFromTables(clause)) {
    const p = pathFromTableToken(t)
    if (!p) continue
    const key = p.join('.')
    if (seen.has(key)) continue
    seen.add(key)
    out.push(p)
  }
  return out
}

function mergedColumnsForPaths(schema: LakeSqlSchema, paths: string[][]): string[] {
  const all = new Set<string>()
  for (const p of paths) {
    const cols = lookupColumns(schema, p)
    if (cols) for (const c of cols) all.add(c)
  }
  return [...all].sort((a, b) => a.localeCompare(b))
}

function inSelectList(doc: string, pos: number): boolean {
  const head = doc.slice(0, pos)
  if (!/\bselect\b/i.test(head)) return false
  const selIdx = findLastMatchIndex(head, /\bselect\b/gi)
  const rest = head.slice(selIdx)
  const fromRel = rest.search(/\bfrom\b/i)
  if (fromRel < 0) return true
  return pos < selIdx + fromRel
}

function kwEnd(doc: string, idx: number, re: RegExp): number {
  const tail = doc.slice(idx)
  const m = tail.match(re)
  return m ? idx + m[0].length : idx
}

function inWhereClause(doc: string, pos: number): boolean {
  const head = doc.slice(0, pos)
  const fromI = findLastMatchIndex(head, /\bfrom\b/gi)
  const whereI = findLastMatchIndex(head, /\bwhere\b/gi)
  if (whereI < 0 || fromI < 0 || whereI <= fromI) return false
  const we = kwEnd(doc, whereI, /^where\b/i)
  if (pos <= we) return false
  const groupI = findLastMatchIndex(head, /\bgroup\s+by\b/gi)
  if (groupI > whereI && pos >= groupI) return false
  return true
}

function inGroupByClause(doc: string, pos: number): boolean {
  const head = doc.slice(0, pos)
  const gbi = findLastMatchIndex(head, /\bgroup\s+by\b/gi)
  if (gbi < 0) return false
  const ge = kwEnd(doc, gbi, /^group\s+by\b/i)
  if (pos <= ge) return false
  const hav = findLastMatchIndex(head, /\bhaving\b/gi)
  const ob = findLastMatchIndex(head, /\border\s+by\b/gi)
  const lim = findLastMatchIndex(head, /\blimit\b/gi)
  const stops = [hav, ob, lim].filter((i) => i > gbi).sort((a, b) => a - b)
  const first = stops[0]
  if (first !== undefined && pos >= first) return false
  return true
}

function inHavingClause(doc: string, pos: number): boolean {
  const head = doc.slice(0, pos)
  const hi = findLastMatchIndex(head, /\bhaving\b/gi)
  if (hi < 0) return false
  const he = kwEnd(doc, hi, /^having\b/i)
  if (pos <= he) return false
  const ob = findLastMatchIndex(head, /\border\s+by\b/gi)
  const lim = findLastMatchIndex(head, /\blimit\b/gi)
  const stops = [ob, lim].filter((i) => i > hi).sort((a, b) => a - b)
  const first = stops[0]
  if (first !== undefined && pos >= first) return false
  return true
}

function inOrderByClause(doc: string, pos: number): boolean {
  const head = doc.slice(0, pos)
  const obi = findLastMatchIndex(head, /\border\s+by\b/gi)
  if (obi < 0) return false
  const oe = kwEnd(doc, obi, /^order\s+by\b/i)
  if (pos <= oe) return false
  const lim = findLastMatchIndex(head, /\blimit\b/gi)
  if (lim > obi && pos >= lim) return false
  return true
}

function bareColumnContext(doc: string, pos: number): boolean {
  if (!/\bselect\b/i.test(doc.slice(0, pos))) return false
  return (
    inSelectList(doc, pos) ||
    inWhereClause(doc, pos) ||
    inGroupByClause(doc, pos) ||
    inHavingClause(doc, pos) ||
    inOrderByClause(doc, pos)
  )
}

/**
 * Column completion after `catalog.schema.table.` including when the table side
 * is a single CompositeIdentifier (Lezer) — built-in sql() schema completion
 * leaves parents empty in that case.
 */
export function lakeQualifiedColumnCompletion(schema: LakeSqlSchema): CompletionSource {
  return (context: CompletionContext) => {
    const ref = context.matchBefore(/([\w]+(?:\.[\w]+)+)\.(\w*)$/)
    if (!ref) return null

    const full = ref.text
    const lastDot = full.lastIndexOf('.')
    const pathPart = full.slice(0, lastDot)
    const partial = full.slice(lastDot + 1)
    const parts = pathPart.split('.').filter(Boolean)
    const cols = lookupColumns(schema, parts)
    if (!cols?.length) return null

    const pl = partial.toLowerCase()
    const options: Completion[] = []
    for (const label of cols) {
      if (pl && !label.toLowerCase().startsWith(pl)) continue
      options.push({ label, type: 'property', boost: 16 })
    }
    if (!options.length) return null

    const from = context.pos - partial.length
    return { from, to: context.pos, options, validFor: /^\w*$/ }
  }
}

/** Columns without table prefix, using tables from FROM / JOIN (and forward FROM for SELECT list). */
export function lakeBareColumnCompletion(schema: LakeSqlSchema): CompletionSource {
  return (context: CompletionContext) => {
    if (context.matchBefore(/([\w]+(?:\.[\w]+)+)\.(\w*)$/)) return null

    const doc = context.state.doc.toString()
    const pos = context.pos
    if (!bareColumnContext(doc, pos)) return null

    const word = context.matchBefore(/\w*$/)
    if (!word && !context.explicit) return null

    const pathsBefore = collectTablePathsFromDocUpTo(doc, pos)
    const pathsSelect = inSelectList(doc, pos) ? collectTablePathsForSelectList(doc, pos) : []
    const seen = new Set<string>()
    const mergedPaths: string[][] = []
    for (const p of [...pathsBefore, ...pathsSelect]) {
      const k = p.join('.')
      if (seen.has(k)) continue
      seen.add(k)
      mergedPaths.push(p)
    }
    if (!mergedPaths.length) return null

    const cols = mergedColumnsForPaths(schema, mergedPaths)
    if (!cols.length) return null

    const partial = word?.text ?? ''
    const pl = partial.toLowerCase()
    const options: Completion[] = []
    for (const label of cols) {
      if (pl && !label.toLowerCase().startsWith(pl)) continue
      options.push({ label, type: 'property', boost: 12 })
    }
    if (!options.length) return null

    const from = word ? word.from : context.pos
    return { from, to: context.pos, options, validFor: /^\w*$/ }
  }
}

/** Qualified `a.b.c.` columns first, then bare columns from FROM in SELECT/WHERE/GROUP BY/HAVING/ORDER BY. */
export function lakeSqlColumnCompletion(schema: LakeSqlSchema): CompletionSource {
  const q = lakeQualifiedColumnCompletion(schema)
  const b = lakeBareColumnCompletion(schema)
  return (context: CompletionContext) => q(context) ?? b(context)
}
