// "Add exercises": search your list by name or muscle, tick one or more,
// or add a new one of your own right here.
import { useMemo, useState } from 'react'
import { api, V1 } from '../lib/api'
import { KINDS, MUSCLES, muscleLabel, useData } from '../lib/data'
import type { Exercise } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { errMsg, Field, Modal, Segmented, useToast } from './ui'

export function ExercisePicker({ onClose, onPick, multi = true, title = 'Add exercises' }: { onClose: () => void; onPick: (ex: Exercise[]) => void; multi?: boolean; title?: string }) {
  const { exercises, reloadExercises } = useData()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const [muscle, setMuscle] = useState('')
  const [picked, setPicked] = useState<Exercise[]>([])
  const [adding, setAdding] = useState(false)
  const [nm, setNm] = useState({ muscle: 'other', kind: 'weight' })
  const list = useMemo(() => {
    const words = q.trim().toLowerCase().split(/\s+/).filter(Boolean)
    return exercises.filter(e => !e.archived && (!muscle || e.muscle === muscle || e.secondary.split(',').includes(muscle)) && words.every(w => `${e.name} ${muscleLabel(e.muscle)}`.toLowerCase().includes(w)))
      .sort((a, b) => (b.sets || 0) - (a.sets || 0) || a.name.localeCompare(b.name))
  }, [exercises, q, muscle])
  const toggle = (e: Exercise) => {
    if (!multi) { onPick([e]); return }
    setPicked(p => p.some(x => x.id === e.id) ? p.filter(x => x.id !== e.id) : [...p, e])
  }
  const create = async () => {
    try {
      const e = await api.post<Exercise>(`${V1}/exercises`, { name: q.trim(), muscle: nm.muscle, kind: nm.kind, equipment: nm.kind === 'cardio' ? 'cardio' : 'other' })
      reloadExercises()
      setAdding(false)
      if (!multi) onPick([e])
      else setPicked(p => [...p, e])
      toast(`${e.name} added to your exercises`)
    } catch (err) { toast(errMsg(err), 'err') }
  }
  return (
    <Modal title={title} onClose={onClose} size="lg" className="picker" footer={multi ? <>
      <span className="muted small grow">{picked.length ? picked.map(p => p.name).join(', ') : 'Tick one or more'}</span>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={!picked.length} onClick={() => onPick(picked)}>Add {picked.length || ''}</button>
    </> : undefined}>
      <div className="stack">
        <input autoFocus value={q} onChange={e => { setQ(e.target.value); setAdding(false) }} placeholder="Search: bench, squat, curl…" aria-label="Search exercises" />
        <div className="chips scroll">
          <button type="button" className={`chip ${!muscle ? 'on' : ''}`} onClick={() => setMuscle('')}>All</button>
          {MUSCLES.map(m => <button key={m.id} type="button" className={`chip ${muscle === m.id ? 'on' : ''}`} onClick={() => setMuscle(muscle === m.id ? '' : m.id)}>{m.label}</button>)}
        </div>
        <div className="pick-list" role="listbox" aria-multiselectable={multi}>
          {list.map(e => {
            const on = picked.some(p => p.id === e.id)
            return (
              <button key={e.id} type="button" role="option" aria-selected={on} className={`pick-row ${on ? 'on' : ''}`} onClick={() => toggle(e)}>
                <span className={`pick-check ${on ? 'on' : ''}`}>{on && <Icon name="check" size={13} strokeWidth={2.6} />}</span>
                <span className="grow ellipsis"><b>{e.name}</b><span className="muted small block-line">{muscleLabel(e.muscle)}{e.sets ? ` · done ${e.sets} sets` : ''}</span></span>
              </button>
            )
          })}
          {!list.length && <p className="muted small pad-sm">No exercise like that{q.trim() ? '; add it as your own below' : ''}.</p>}
        </div>
        {q.trim().length > 1 && !exercises.some(e => e.name.toLowerCase() === q.trim().toLowerCase()) && (adding
          ? <div className="card inset"><div className="card-body stack">
              <b>New exercise: {q.trim()}</b>
              <div className="grid two-grid">
                <Field label="Main muscle" group><Select value={nm.muscle} onChange={v => setNm({ ...nm, muscle: v })} options={MUSCLES.map(m => ({ value: m.id, label: m.label }))} /></Field>
                <Field label="Each set is" group><Segmented size="sm" value={nm.kind} onChange={v => setNm({ ...nm, kind: v })} options={KINDS.map(k => ({ value: k.id, label: k.label.split(' ')[0], title: k.help }))} /></Field>
              </div>
              <div className="row"><button type="button" className="btn primary" onClick={create}>Add it</button><button type="button" className="btn ghost" onClick={() => setAdding(false)}>Cancel</button></div>
            </div></div>
          : <button type="button" className="btn ghost self-start" onClick={() => setAdding(true)}><Icon name="plus" size={15} />Add “{q.trim()}” as a new exercise</button>)}
      </div>
    </Modal>
  )
}
