import { Md5 } from 'ts-md5'

const MAX_CACHE_ENTRIES = 500

function setCacheEntry<K, V>(map: Map<K, V>, key: K, value: V) {
  if (!map.has(key) && map.size >= MAX_CACHE_ENTRIES) {
    const oldestKey = map.keys().next().value
    if (oldestKey !== undefined) map.delete(oldestKey)
  }
  map.set(key, value)
}

function md5(str: string): string {
  return Md5.hashAsciiStr(str || '') + ''
}

const colorHexCache = new Map<string, string>()

function getColorByStringHEX(str: string): string {
  const cached = colorHexCache.get(str)
  if (cached) return cached

  let hash = 0
  const hashed = md5(str)
  for (let i = 0; i < hashed.length; i++) {
    hash = hashed.charCodeAt(i) + ((hash << 5) - hash)
  }

  let col =
    ((hash >> 24) & 0xaf).toString(16) +
    ((hash >> 16) & 0xaf).toString(16) +
    ((hash >> 8) & 0xaf).toString(16) +
    (hash & 0xaf).toString(16)

  if (col.length < 6) col = col.substring(0, 3) + col.substring(0, 3)
  if (col.length > 6) col = col.substring(0, 6)

  setCacheEntry(colorHexCache, str, col)
  return col
}

const hueCache = new Map<string, number>()

function getHueFromHex(col: string): number | null {
  const cached = hueCache.get(col)
  if (cached !== undefined) return cached

  const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(col)
  if (!result) return null

  const r = parseInt(result[1], 16) / 255
  const g = parseInt(result[2], 16) / 255
  const b = parseInt(result[3], 16) / 255

  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  let h = 0

  if (max !== min) {
    const d = max - min
    switch (max) {
      case r:
        h = (g - b) / d + (g < b ? 6 : 0)
        break
      case g:
        h = (b - r) / d + 2
        break
      case b:
        h = (r - g) / d + 4
        break
    }
    h /= 6
  }

  h = Math.round(h * 360)
  setCacheEntry(hueCache, col, h)
  return h
}

export function getColorByString(
  str: string,
  saturation = 60,
  lightness = 60,
  alpha = 1,
  offset = 0,
): string {
  const col = getColorByStringHEX(str)
  const h = getHueFromHex(col)
  if (h === null) return `hsla(0, ${saturation}%, ${lightness}%, ${alpha})`
  return `hsla(${h - offset}, ${saturation}%, ${lightness}%, ${alpha})`
}

export function getMethodColor(str: unknown): string {
  let color = 'hsla(0, 0%, 46%, 1)'
  const raw = ((str ?? '') + '').replace(/\s*\(.*/g, '')

  if (raw === 'INVITE' || raw === 're-INVITE') {
    color = 'hsla(227.5, 82.4%, 51%, 1)'
  } else if (raw === 'BYE' || raw === 'CANCEL') {
    color = 'hsla(120, 100%, 25%, 1)'
  } else if (raw.startsWith('RTCP')) {
    color = 'hsla(187, 72%, 38%, 1)'
  } else {
    const code = Number(raw)
    if (Number.isFinite(code)) {
      if (code >= 100 && code < 200) color = 'hsla(0, 0%, 44%, 1)'
      else if (code >= 200 && code < 300) color = 'hsla(120, 70%, 50%, 1)'
      else if (code >= 300 && code < 400) color = 'hsla(280, 100%, 50%, 1)'
      else if (code >= 400 && code < 500) color = 'hsla(15, 100%, 45%, 1)'
      else if (code >= 500 && code < 700) color = 'hsla(0, 100%, 45%, 1)'
    }
  }
  return color
}

interface HslComponents {
  hue: number
  saturation: number
  lightness: number
}

function parseHslColor(s: string): HslComponents | null {
  const m = s.match(/hsla?\((-?\d+),\s*(\d+)%,\s*(\d+)%/)
  if (!m) return null
  return { hue: parseInt(m[1]), saturation: parseInt(m[2]), lightness: parseInt(m[3]) }
}

export interface CallIdColors {
  color: string
  tabColor: string
  arrowColor: string
}

export function getHighContrastColor(
  baseCallId: string,
  currentCallId: string,
  allCallIds: string[],
): CallIdColors {
  const baseColor = getColorByString(baseCallId, 60, 60)
  const baseComponents = parseHslColor(baseColor)

  if (!baseComponents) {
    const fallback = getColorByString(currentCallId, 60, 60)
    return { color: fallback, tabColor: fallback, arrowColor: fallback }
  }

  const index = Math.max(0, allCallIds.indexOf(currentCallId))

  let offsetStep = 360 / Math.max(1, allCallIds.length)
  if (offsetStep % 360 === 0) offsetStep += 14

  const hue = index === 0 ? baseComponents.hue : baseComponents.hue + offsetStep * index
  const saturation = index === 0 ? baseComponents.saturation + 10 : 60
  const lightness = index === 0 ? baseComponents.lightness - 20 : 70

  return {
    color: `hsla(${hue}, ${saturation}%, ${lightness}%, 0.2)`,
    tabColor: `hsla(${hue}, ${saturation}%, ${lightness}%, 1)`,
    arrowColor: `hsla(${hue}, ${saturation}%, ${lightness}%, 0.55)`,
  }
}
