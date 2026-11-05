// Weights are kept in kg and distances in metres; these show them in the
// person's units and read what they type back.
import type { Exercise, Settings, WSet, WeightUnit, DistanceUnit, RecordKind } from './types'

export const LB = 2.20462262

export interface Units { w: WeightUnit; d: DistanceUnit }
export const unitsOf = (s: Settings | null | undefined): Units => ({ w: s?.weight_unit || 'kg', d: s?.distance_unit || 'km' })

/** kg → the number shown (rounded to what a plate allows). */
export function wOut(kg: number | null | undefined, u: Units): number | null {
  if (kg == null) return null
  return u.w === 'lb' ? Math.round(kg * LB * 10) / 10 : Math.round(kg * 100) / 100
}
/** The number typed → kg. */
export function wIn(v: number | null, u: Units): number | null {
  if (v == null || isNaN(v)) return null
  return u.w === 'lb' ? Math.round(v / LB * 1000) / 1000 : v
}
export const num = (v: number, max = 1) => v.toLocaleString(undefined, { maximumFractionDigits: max })
export const weight = (kg: number | null | undefined, u: Units) => kg == null ? '' : `${num(wOut(kg, u)!)} ${u.w}`
/** A big total: "12,340 kg". */
export const tonnage = (kg: number, u: Units) => `${Math.round(u.w === 'lb' ? kg * LB : kg).toLocaleString()} ${u.w}`

export function dOut(m: number | null | undefined, u: Units): number | null {
  if (m == null) return null
  return u.d === 'mi' ? Math.round(m / 1609.344 * 100) / 100 : Math.round(m / 10) / 100
}
export function dIn(v: number | null, u: Units): number | null {
  if (v == null || isNaN(v)) return null
  return u.d === 'mi' ? Math.round(v * 1609.344) : Math.round(v * 1000)
}
export const distance = (m: number | null | undefined, u: Units) => m == null ? '' : `${num(dOut(m, u)!, 2)} ${u.d}`

/** Seconds as 1:05 or 1:02:05. */
export function clock(sec: number | null | undefined): string {
  if (sec == null) return ''
  const s = Math.max(0, Math.round(sec))
  const h = Math.floor(s / 3600), m = Math.floor(s / 60) % 60, r = s % 60
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(r).padStart(2, '0')}` : `${m}:${String(r).padStart(2, '0')}`
}
/** "1:30" or "90" → 90 seconds. */
export function parseClock(v: string): number | null {
  v = v.trim()
  if (!v) return null
  if (/^\d+(\.\d+)?$/.test(v)) return Math.round(Number(v))
  const p = v.split(':').map(Number)
  if (p.some(isNaN)) return null
  return p.reduce((a, b) => a * 60 + b, 0)
}
/** Minutes as "1 h 5 min". */
export function mins(m: number): string {
  if (m < 60) return `${m} min`
  return `${Math.floor(m / 60)} h${m % 60 ? ` ${m % 60} min` : ''}`
}

/** A set in words: "80 kg × 8", "12 reps", "1:30", "5.2 km in 28:10". */
export function setText(s: Pick<WSet, 'weight' | 'reps' | 'duration_s' | 'distance_m'>, ex: Exercise | undefined, u: Units): string {
  const kind = ex?.kind || 'weight'
  if (kind === 'cardio' || (s.distance_m && s.distance_m > 0)) return [s.distance_m ? distance(s.distance_m, u) : '', s.duration_s ? (s.distance_m ? 'in ' : '') + clock(s.duration_s) : ''].filter(Boolean).join(' ')
  if (kind === 'time') return clock(s.duration_s)
  if (kind === 'reps') return `${s.reps ?? 0} reps${s.weight ? ` +${weight(s.weight, u)}` : ''}`
  if (s.weight != null && s.reps != null) return `${weight(s.weight, u)} × ${s.reps}`
  if (s.reps != null) return `${s.reps} reps`
  return ''
}

/** Estimated one-rep max (Epley), 1–12 reps. */
export function e1rm(s: Pick<WSet, 'weight' | 'reps'>): number {
  if (!s.weight || !s.reps || s.reps < 1 || s.reps > 12) return 0
  return s.reps === 1 ? s.weight : s.weight * (1 + s.reps / 30)
}

export const RECORD_LABEL: Record<RecordKind, string> = { weight: 'Heaviest', e1rm: 'Best estimated max', volume: 'Most in one set', reps: 'Most reps', duration: 'Longest', distance: 'Furthest' }

/** The plates for each side of a bar ("20 + 10 + 2.5"), in kg or lb. */
export function plates(totalKg: number, barKg: number, u: Units): string {
  const unitPlates = u.w === 'lb' ? [45, 35, 25, 10, 5, 2.5] : [25, 20, 15, 10, 5, 2.5, 1.25]
  let side = ((u.w === 'lb' ? totalKg * LB : totalKg) - (u.w === 'lb' ? barKg * LB : barKg)) / 2
  if (side <= 0.01) return 'just the bar'
  const out: number[] = []
  for (const p of unitPlates) while (side >= p - 0.01) { out.push(p); side -= p }
  return out.length ? out.join(' + ') + (side > 0.2 ? ` (+${num(side)})` : '') : ''
}
