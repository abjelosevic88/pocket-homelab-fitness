// Number fields that let people type freely ("82.5", "1:30") and only
// read the value back as a number, in the person's units.
import { useEffect, useRef, useState } from 'react'
import type { Exercise, WSet } from '../lib/types'
import { clock, dIn, dOut, parseClock, type Units, wIn, wOut } from '../lib/units'

/** A decimal field: shows `value`, reports the number typed (null when empty). */
export function NumInput({ value, onChange, placeholder, label, className = '', step, onEnter }: { value: number | null; onChange: (v: number | null) => void; placeholder?: string; label: string; className?: string; step?: number; onEnter?: () => void }) {
  const [text, setText] = useState(value == null ? '' : String(value))
  const focused = useRef(false)
  useEffect(() => { if (!focused.current) setText(value == null ? '' : String(value)) }, [value])
  return (
    <input className={`num-in ${className}`} inputMode="decimal" aria-label={label} placeholder={placeholder} value={text}
      onFocus={e => { focused.current = true; e.currentTarget.select() }}
      onBlur={() => { focused.current = false; setText(value == null ? '' : String(value)) }}
      onKeyDown={e => {
        if (e.key === 'Enter') { e.preventDefault(); onEnter?.() }
        if (step && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) { e.preventDefault(); onChange(Math.max(0, Math.round(((value || 0) + (e.key === 'ArrowUp' ? step : -step)) * 100) / 100)) }
      }}
      onChange={e => {
        const t = e.target.value.replace(',', '.')
        if (!/^\d*\.?\d*$/.test(t)) return
        setText(t)
        onChange(t === '' || t === '.' ? null : Number(t))
      }} />
  )
}

/** A time field: "1:30" or "90" (seconds). */
export function ClockInput({ value, onChange, label, placeholder = '0:00', className = '' }: { value: number | null; onChange: (v: number | null) => void; label: string; placeholder?: string; className?: string }) {
  const [text, setText] = useState(value == null ? '' : clock(value))
  const focused = useRef(false)
  useEffect(() => { if (!focused.current) setText(value == null ? '' : clock(value)) }, [value])
  return (
    <input className={`num-in ${className}`} inputMode="numeric" aria-label={label} placeholder={placeholder} value={text}
      onFocus={e => { focused.current = true; e.currentTarget.select() }}
      onBlur={() => { focused.current = false; setText(value == null ? '' : clock(value)) }}
      onChange={e => { const t = e.target.value.replace(/[^\d:]/g, ''); setText(t); onChange(parseClock(t)) }} />
  )
}

type SetVals = Pick<WSet, 'weight' | 'reps' | 'duration_s' | 'distance_m'>

/** The fields of one set, by the exercise's kind. */
export function SetFields({ set, ex, u, onChange, onEnter }: { set: SetVals; ex: Exercise | undefined; u: Units; onChange: (p: Partial<SetVals>) => void; onEnter?: () => void }) {
  const kind = ex?.kind || 'weight'
  const step = u.w === 'lb' ? 5 : 2.5
  if (kind === 'cardio') return (
    <div className="set-fields">
      <NumInput label={`Distance in ${u.d}`} placeholder={u.d} value={dOut(set.distance_m, u)} onChange={v => onChange({ distance_m: dIn(v, u) })} onEnter={onEnter} />
      <ClockInput label="Time" value={set.duration_s} onChange={v => onChange({ duration_s: v })} />
    </div>
  )
  if (kind === 'time') return (
    <div className="set-fields">
      <ClockInput label="Time" value={set.duration_s} onChange={v => onChange({ duration_s: v })} />
      <NumInput label={`Extra weight in ${u.w}`} placeholder={`+${u.w}`} value={wOut(set.weight, u)} onChange={v => onChange({ weight: wIn(v, u) })} onEnter={onEnter} />
    </div>
  )
  if (kind === 'reps') return (
    <div className="set-fields">
      <NumInput label="Reps" placeholder="reps" value={set.reps} step={1} onChange={v => onChange({ reps: v == null ? null : Math.round(v) })} onEnter={onEnter} />
      <NumInput label={`Extra weight in ${u.w}`} placeholder={`+${u.w}`} value={wOut(set.weight, u)} step={step} onChange={v => onChange({ weight: wIn(v, u) })} onEnter={onEnter} />
    </div>
  )
  return (
    <div className="set-fields">
      <NumInput label={`Weight in ${u.w}`} placeholder={u.w} value={wOut(set.weight, u)} step={step} onChange={v => onChange({ weight: wIn(v, u) })} onEnter={onEnter} />
      <NumInput label="Reps" placeholder="reps" value={set.reps} step={1} onChange={v => onChange({ reps: v == null ? null : Math.round(v) })} onEnter={onEnter} />
    </div>
  )
}

/** Column headings for SetFields. */
export function setHeads(ex: Exercise | undefined, u: Units): [string, string] {
  switch (ex?.kind) {
    case 'cardio': return [u.d, 'time']
    case 'time': return ['time', `+${u.w}`]
    case 'reps': return ['reps', `+${u.w}`]
  }
  return [u.w, 'reps']
}
