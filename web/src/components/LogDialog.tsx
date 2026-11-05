// "Log a workout": one you already did, typed in set by set, or written in
// a sentence that Pocket AI turns into exercises and sets to check first.
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { todayISO } from '../lib/format'
import type { Exercise, Finished } from '../lib/types'
import { unitsOf } from '../lib/units'
import { ExercisePicker } from './ExercisePicker'
import { Icon } from './icons'
import { NumInput, SetFields, setHeads } from './Inputs'
import { errMsg, Field, Modal, Spinner, useToast } from './ui'

type DSet = { weight: number | null; reps: number | null; duration_s: number | null; distance_m: number | null }
type DEx = { key: number; exercise_id: number; name: string; kind: string; isNew?: boolean; sets: DSet[] }
const blank = (): DSet => ({ weight: null, reps: null, duration_s: null, distance_m: null })
let seq = 1

export function LogDialog({ onClose }: { onClose: () => void }) {
  const { settings } = useApp()
  const { byId } = useData()
  const u = unitsOf(settings)
  const { toast } = useToast()
  const navigate = useNavigate()
  const [text, setText] = useState('')
  const [ai, setAI] = useState<{ ready: boolean; detail: string } | null>(null)
  const [reading, setReading] = useState(false)
  const [name, setName] = useState('')
  const [date, setDate] = useState(todayISO())
  const [minutes, setMinutes] = useState<number | null>(null)
  const [exs, setExs] = useState<DEx[]>([])
  const [picking, setPicking] = useState(false)
  const [saving, setSaving] = useState(false)
  useEffect(() => { api.get<{ ready: boolean; detail: string }>(`${V1}/ai/status`).then(setAI).catch(() => setAI({ ready: false, detail: '' })) }, [])

  const understand = async () => {
    setReading(true)
    try {
      const r = await api.post<{ ok: boolean; error?: string; name: string; date: string; duration_min: number | null; exercises: { exercise_id: number; name: string; kind: string; new: boolean; sets: DSet[] }[] }>(`${V1}/ai/parse`, { text })
      if (!r.ok) { toast(r.error || 'That did not work', 'err'); return }
      if (r.name) setName(r.name)
      if (r.date) setDate(r.date)
      if (r.duration_min) setMinutes(r.duration_min)
      setExs(r.exercises.map(e => ({ key: seq++, exercise_id: e.exercise_id, name: e.name, kind: e.kind, isNew: e.new, sets: e.sets.map(s => ({ ...blank(), ...s })) })))
    } catch (e) { toast(errMsg(e), 'err') } finally { setReading(false) }
  }
  const add = (list: Exercise[]) => {
    setPicking(false)
    setExs(x => [...x, ...list.map(e => ({ key: seq++, exercise_id: e.id, name: e.name, kind: e.kind, sets: [blank(), blank(), blank()] }))])
  }
  const patch = (k: number, f: (e: DEx) => DEx) => setExs(x => x.map(e => e.key === k ? f(e) : e))
  const save = async () => {
    setSaving(true)
    try {
      const body = { name, date, duration_min: minutes || 0, exercises: exs.map(e => ({ exercise_id: e.isNew ? 0 : e.exercise_id, name: e.name, kind: e.kind, sets: e.sets })) }
      const r = await api.post<Finished>(`${V1}/workouts/log`, body)
      announceChange()
      toast(r.records.length ? `Logged, with ${r.records.length} new ${r.records.length === 1 ? 'record' : 'records'}` : 'Workout logged')
      onClose()
      navigate(`/workouts/${r.workout.id}`)
    } catch (e) { toast(errMsg(e), 'err') } finally { setSaving(false) }
  }
  const exOf = (e: DEx): Exercise | undefined => byId[e.exercise_id] || ({ kind: e.kind } as Exercise)
  return (
    <Modal title="Log a workout" onClose={onClose} size="lg" footer={<>
      <span className="grow" />
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" disabled={!exs.length || saving} onClick={save}>{saving ? <Spinner /> : <Icon name="check" size={15} />}Log it</button>
    </>}>
      <div className="stack loose">
        <div className="stack">
          <Field label="Write what you did" help={ai && !ai.ready ? <>Understanding a sentence needs Pocket AI or an OpenAI key (Settings → AI){ai.detail ? `: ${ai.detail}` : ''}. You can add the exercises below by hand.</> : 'For example: bench 3×8 at 80, then 3 sets of 10 pull-ups and a 20 minute run.'}>
            <textarea rows={2} value={text} onChange={e => setText(e.target.value)} placeholder="Bench 3x8 at 80, pull-ups 3x10, 5 km run in 27 minutes" disabled={!ai?.ready} />
          </Field>
          <button type="button" className="btn self-start" disabled={!ai?.ready || text.trim().length < 3 || reading} onClick={understand}>{reading ? <Spinner /> : <Icon name="sparkles" size={15} />}{reading ? 'Reading it…' : 'Fill it in from that'}</button>
        </div>
        <div className="grid three-grid">
          <Field label="Name"><input value={name} onChange={e => setName(e.target.value)} placeholder="Evening workout" /></Field>
          <Field label="Day"><input type="date" value={date} max={todayISO()} onChange={e => setDate(e.target.value)} /></Field>
          <Field label="Minutes" group><NumInput label="Minutes" value={minutes} onChange={setMinutes} placeholder="about" /></Field>
        </div>
        {exs.map(e => {
          const ex = exOf(e)
          const [h1, h2] = setHeads(ex, u)
          return (
            <div key={e.key} className="log-ex">
              <div className="row"><b className="grow">{e.name}</b>{e.isNew && <span className="pill">new exercise</span>}
                <button type="button" className="btn ghost icon-only sm" aria-label={`Remove ${e.name}`} onClick={() => setExs(x => x.filter(y => y.key !== e.key))}><Icon name="trash" size={15} /></button></div>
              <div className="set-grid log">
                <span className="set-h">Set</span><span className="set-h">{h1}</span><span className="set-h">{h2}</span><span />
                {e.sets.map((s, i) => [
                  <span key={`n${i}`} className="set-no">{i + 1}</span>,
                  <SetFields key={`f${i}`} set={s} ex={ex} u={u} onChange={p => patch(e.key, x => ({ ...x, sets: x.sets.map((y, j) => j === i ? { ...y, ...p } : y) }))} />,
                  <button key={`d${i}`} type="button" className="btn ghost icon-only sm" aria-label="Remove this set" onClick={() => patch(e.key, x => ({ ...x, sets: x.sets.filter((_, j) => j !== i) }))}><Icon name="x" size={14} /></button>,
                ])}
              </div>
              <button type="button" className="link-btn small" onClick={() => patch(e.key, x => ({ ...x, sets: [...x.sets, { ...(x.sets[x.sets.length - 1] || blank()) }] }))}>+ Add a set</button>
            </div>
          )
        })}
        <button type="button" className="btn self-start" onClick={() => setPicking(true)}><Icon name="plus" size={15} />Add exercises</button>
      </div>
      {picking && <ExercisePicker onClose={() => setPicking(false)} onPick={add} />}
    </Modal>
  )
}
