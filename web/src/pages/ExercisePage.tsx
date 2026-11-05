// One exercise: your records, the best of each session over time, and the
// sessions themselves.
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, EQUIPMENT, muscleLabel } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Exercise, RecordRow, Session } from '../lib/types'
import { clock, distance, RECORD_LABEL, setText, tonnage, unitsOf, weight } from '../lib/units'
import { useCrumb } from '../components/Layout'
import { LineChart } from '../components/Charts'
import { Icon } from '../components/icons'
import { Card, Empty, errMsg, Loading, MenuButton, Segmented, useToast } from '../components/ui'
import { ExerciseForm } from './Exercises'

interface Detail { exercise: Exercise; records: RecordRow[]; history: Session[]; chart: { date: string; best: number; e1rm: number; volume: number }[] }

export default function ExercisePage() {
  const { id } = useParams()
  const { settings } = useApp()
  const u = unitsOf(settings)
  const { toast } = useToast()
  const [d, setD] = useState<Detail | null>(null)
  const [err, setErr] = useState('')
  const [edit, setEdit] = useState(false)
  const [line, setLine] = useState<'best' | 'e1rm' | 'volume'>('best')
  const load = useCallback(() => {
    api.get<Detail>(`${V1}/exercises/${id}`).then(x => { setD(x); setErr('') }).catch(e => setErr(e instanceof ApiError && e.status === 404 ? 'notfound' : errMsg(e)))
  }, [id])
  useEffect(load, [load])
  useCrumb(d?.exercise.name, 'exercises')
  if (err === 'notfound') return <Empty title="No such exercise" icon="dumbbell"><Link to="/exercises">All exercises</Link></Empty>
  if (err) return <div className="callout error">{err}</div>
  if (!d) return <Loading />
  const e = d.exercise
  const fmtRec = (r: RecordRow) => {
    switch (r.kind) {
      case 'weight': return `${weight(r.value, u)} × ${r.reps}`
      case 'volume': return `${weight(r.weight, u)} × ${r.reps}`
      case 'e1rm': return weight(r.value, u)
      case 'reps': return `${r.value} reps`
      case 'duration': return clock(r.value)
      case 'distance': return distance(r.value, u)
    }
  }
  const isWeight = e.kind === 'weight'
  const fmtLine = (v: number) => line === 'volume' ? tonnage(v, u) : isWeight || line === 'e1rm' ? weight(v, u) : e.kind === 'time' ? clock(v) : e.kind === 'cardio' ? distance(v, u) : `${v} reps`
  const hide = async () => {
    try { await api.put(`${V1}/exercises/${e.id}`, { archived: !e.archived }); announceChange(); load(); toast(e.archived ? 'Back in your list' : 'Hidden from your lists; its history stays') } catch (x) { toast(errMsg(x), 'err') }
  }
  return (
    <div className="stack loose">
      <div className="detail-head">
        <span className="detail-ico"><Icon name={e.kind === 'cardio' ? 'run' : e.kind === 'time' ? 'timer' : 'dumbbell'} size={22} /></span>
        <div className="grow col"><h1 className="page-title">{e.name}</h1>
          <p className="muted small">{muscleLabel(e.muscle)}{e.secondary ? ` (also ${e.secondary.split(',').map(muscleLabel).join(', ').toLowerCase()})` : ''} · {EQUIPMENT.find(x => x.id === e.equipment)?.label}{e.archived ? ' · hidden' : ''}</p></div>
        <div className="row toolbar">
          <button type="button" className="btn" onClick={() => setEdit(true)}><Icon name="edit" size={15} />Change</button>
          <MenuButton label="More" align="right" items={[{ label: e.archived ? 'Show it in your lists again' : 'Hide it from your lists', icon: e.archived ? 'eye' : 'eye-off', onSelect: hide }]}><Icon name="more" size={18} /></MenuButton>
        </div>
      </div>
      {e.notes && <p className="notes">{e.notes}</p>}
      {!d.history.length ? <Card><Empty title="Not done yet" icon="dumbbell">Your records and progress for {e.name} show up here once you do it in a workout.</Empty></Card> : <>
        <div className="stats-row">{d.records.map(r => (
          <Link key={r.kind} to={`/workouts/${r.workout_id}`} className="stat stat-link"><span className="stat-l">{RECORD_LABEL[r.kind]}</span><span className="stat-v">{fmtRec(r)}</span><span className="stat-s small">{fmtDate(r.date, false)}</span></Link>
        ))}</div>
        <Card title="Over time" actions={isWeight ? <Segmented size="sm" value={line} onChange={setLine} options={[{ value: 'best', label: 'Heaviest' }, { value: 'e1rm', label: 'Est. max' }, { value: 'volume', label: 'Volume' }]} /> : undefined}>
          <LineChart label={e.name} fmt={fmtLine} points={d.chart.map(c => ({ date: c.date, v: line === 'e1rm' ? c.e1rm : line === 'volume' ? c.volume : c.best })).filter(p => p.v > 0)} />
        </Card>
        <Card title="Sessions" flush><ul className="list">{d.history.map(s => (
          <li key={s.workout_id}><Link to={`/workouts/${s.workout_id}`} className="list-row">
            <span className="row-main grow"><span className="row-title">{fmtDate(s.date)} <span className="muted small">{s.workout}</span></span>
              <span className="small sets-line">{s.sets.filter(x => x.type !== 'warmup').map((x, i) => <span key={i} className={x.pr?.length ? 'is-pr' : ''}>{setText(x, e, u)}{x.pr?.length ? <Icon name="trophy" size={11} /> : null}</span>)}</span></span>
            {s.e1rm > 0 && <span className="muted small hide-xs right">est. max {weight(s.e1rm, u)}</span>}
          </Link></li>))}</ul></Card>
      </>}
      {edit && <ExerciseForm initial={e} onClose={() => setEdit(false)} onSaved={() => load()} />}
    </div>
  )
}
