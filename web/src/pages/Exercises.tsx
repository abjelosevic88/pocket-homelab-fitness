// The exercise list: the built-in ones and your own, by muscle, with how
// often you did them; add, rename, hide.
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, EQUIPMENT, KINDS, MUSCLES, muscleLabel, useData } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Exercise } from '../lib/types'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Empty, errMsg, Field, Loading, Modal, Segmented, useToast } from '../components/ui'

export default function Exercises() {
  const { exercises } = useData()
  const [q, setQ] = useState('')
  const [muscle, setMuscle] = useState('')
  const [show, setShow] = useState<'all' | 'done' | 'hidden'>('all')
  const [editing, setEditing] = useState<Partial<Exercise> | null>(null)
  const list = useMemo(() => {
    const words = q.trim().toLowerCase().split(/\s+/).filter(Boolean)
    return exercises.filter(e => (show === 'hidden' ? e.archived : !e.archived) && (show !== 'done' || (e.sets || 0) > 0)
      && (!muscle || e.muscle === muscle) && words.every(w => `${e.name} ${muscleLabel(e.muscle)} ${e.equipment}`.toLowerCase().includes(w)))
  }, [exercises, q, muscle, show])
  const counts = useMemo(() => Object.fromEntries(MUSCLES.map(m => [m.id, exercises.filter(e => !e.archived && e.muscle === m.id).length])), [exercises])
  if (!exercises.length) return <Loading />
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Exercises</h1><p className="muted">{exercises.filter(e => !e.archived).length} exercises; {exercises.filter(e => (e.sets || 0) > 0).length} you have done.</p></div>
        <button type="button" className="btn primary" onClick={() => setEditing({ name: q, muscle: (muscle || 'other') as Exercise['muscle'], kind: 'weight', equipment: 'other' })}><Icon name="plus" size={15} />New exercise</button>
      </div>
      <div className="row wrap filters">
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder="Search exercises" aria-label="Search exercises" />
        <Segmented size="sm" value={show} onChange={setShow} options={[{ value: 'all', label: 'All' }, { value: 'done', label: 'Done' }, { value: 'hidden', label: 'Hidden' }]} />
      </div>
      <div className="chips scroll">
        <button type="button" className={`chip ${!muscle ? 'on' : ''}`} onClick={() => setMuscle('')}>All</button>
        {MUSCLES.filter(m => counts[m.id]).map(m => <button key={m.id} type="button" className={`chip ${muscle === m.id ? 'on' : ''}`} onClick={() => setMuscle(muscle === m.id ? '' : m.id)}>{m.label}<span className="n">{counts[m.id]}</span></button>)}
      </div>
      <Card flush>{list.length ? <ul className="list">{list.map(e => (
        <li key={e.id}><Link to={`/exercises/${e.id}`} className={`list-row ${e.archived ? 'is-archived' : ''}`}>
          <span className="list-ico"><Icon name={e.kind === 'cardio' ? 'run' : e.kind === 'time' ? 'timer' : 'dumbbell'} size={16} /></span>
          <span className="row-main grow"><span className="row-title">{e.name}{!e.builtin && <span className="pill">yours</span>}</span>
            <span className="muted small">{muscleLabel(e.muscle)} · {EQUIPMENT.find(x => x.id === e.equipment)?.label}</span></span>
          <span className="muted small right">{e.sets ? <>{e.sets} sets<span className="block-line">last {fmtDate(e.last_done!, false)}</span></> : 'not done yet'}</span>
        </Link></li>))}</ul> : <Empty title="No exercise like that" icon="search">Add it as your own with New exercise.</Empty>}</Card>
      {editing && <ExerciseForm initial={editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

/** Add or change an exercise. */
export function ExerciseForm({ initial, onClose, onSaved }: { initial: Partial<Exercise>; onClose: () => void; onSaved?: (e: Exercise) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState<Partial<Exercise>>(initial)
  const save = async () => {
    try {
      const body = { name: f.name, muscle: f.muscle, secondary: f.secondary || '', equipment: f.equipment, kind: f.kind, notes: f.notes || '' }
      const e = f.id ? await api.put<Exercise>(`${V1}/exercises/${f.id}`, body) : await api.post<Exercise>(`${V1}/exercises`, body)
      announceChange(); toast(f.id ? 'Saved' : `${e.name} added`); onSaved?.(e); onClose()
    } catch (e) { toast(errMsg(e), 'err') }
  }
  return (
    <Modal title={f.id ? 'Change exercise' : 'New exercise'} onClose={onClose} footer={<><button type="button" className="btn" onClick={onClose}>Cancel</button><button type="button" className="btn primary" disabled={!f.name?.trim()} onClick={save}>Save</button></>}>
      <div className="stack loose form-grid">
        <Field label="Name"><input autoFocus value={f.name || ''} onChange={e => setF({ ...f, name: e.target.value })} placeholder="Sled push" /></Field>
        <div className="grid two-grid">
          <Field label="Main muscle" group><Select value={f.muscle || 'other'} onChange={v => setF({ ...f, muscle: v as Exercise['muscle'] })} options={MUSCLES.map(m => ({ value: m.id, label: m.label }))} /></Field>
          <Field label="Equipment" group><Select value={f.equipment || 'other'} onChange={v => setF({ ...f, equipment: v as Exercise['equipment'] })} options={EQUIPMENT.map(m => ({ value: m.id, label: m.label }))} /></Field>
        </div>
        <Field label="Also works" group help="Counted as half a set each in your muscle balance."><div className="chips">{MUSCLES.filter(m => m.id !== f.muscle && m.id !== 'other').map(m => {
          const on = (f.secondary || '').split(',').includes(m.id)
          return <button key={m.id} type="button" className={`chip ${on ? 'on' : ''}`} onClick={() => { const s = (f.secondary || '').split(',').filter(Boolean); setF({ ...f, secondary: (on ? s.filter(x => x !== m.id) : [...s, m.id]).join(',') }) }}>{m.label}</button>
        })}</div></Field>
        <Field label="Each set is" group help={KINDS.find(k => k.id === f.kind)?.help}><Segmented value={f.kind || 'weight'} onChange={v => setF({ ...f, kind: v as Exercise['kind'] })} options={KINDS.map(k => ({ value: k.id, label: k.label }))} /></Field>
        <Field label="Notes"><textarea rows={2} value={f.notes || ''} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="Seat at 4, grip just outside the shoulders" /></Field>
      </div>
    </Modal>
  )
}
