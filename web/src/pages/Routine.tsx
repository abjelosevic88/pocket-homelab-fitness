// One routine: its exercises with what to aim for, editable in place, the
// days it is planned on, and the last times it was done.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, muscleLabel } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Exercise, Routine, RoutineItem, Workout } from '../lib/types'
import { clock, distance, dIn, dOut, mins, tonnage, unitsOf, weight, wIn, wOut } from '../lib/units'
import { useCrumb } from '../components/Layout'
import { useStart } from '../components/StartSheet'
import { ExercisePicker } from '../components/ExercisePicker'
import { ClockInput, NumInput } from '../components/Inputs'
import { Icon } from '../components/icons'
import { Card, Confirm, Empty, errMsg, Loading, MenuButton, Spinner, Switch, useToast } from '../components/ui'

const DAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']

function aim(it: RoutineItem, ex: Exercise | undefined, u: ReturnType<typeof unitsOf>): string {
  const reps = it.reps_min && it.reps_max && it.reps_min !== it.reps_max ? `${it.reps_min}–${it.reps_max}` : String(it.reps_max || it.reps_min || '')
  switch (ex?.kind) {
    case 'time': return `${it.sets} × ${clock(it.duration_s) || '–'}`
    case 'cardio': return `${it.sets} × ${[it.distance_m ? distance(it.distance_m, u) : '', it.duration_s ? clock(it.duration_s) : ''].filter(Boolean).join(' in ') || '–'}`
  }
  return `${it.sets} × ${reps || '–'}${it.weight ? ` at ${weight(it.weight, u)}` : ''}`
}

export default function RoutinePage() {
  const { id } = useParams()
  const [sp, setSp] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const u = unitsOf(settings)
  const { start, busy } = useStart()
  const [r, setR] = useState<Routine | null>(null)
  const [err, setErr] = useState('')
  const [draft, setDraft] = useState<Routine | null>(null)
  const [picking, setPicking] = useState(false)
  const [saving, setSaving] = useState(false)
  const [del, setDel] = useState(false)
  const [recent, setRecent] = useState<Workout[]>([])
  const load = useCallback(() => {
    api.get<Routine>(`${V1}/routines/${id}`).then(x => { setR(x); setErr(''); if (sp.get('edit')) { setDraft(x); setSp(new URLSearchParams(), { replace: true }) } })
      .catch(e => setErr(e instanceof ApiError && e.status === 404 ? 'notfound' : errMsg(e)))
    api.get<Workout[]>(`${V1}/workouts?routine=${id}&limit=8`).then(setRecent).catch(() => {})
  }, [id]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(load, [load])
  useCrumb(r?.name, 'routines')
  if (err === 'notfound') return <Empty title="No such routine" icon="list"><Link to="/routines">Your routines</Link></Empty>
  if (err) return <div className="callout error">{err}</div>
  if (!r) return <Loading />
  const d = draft
  const items = (d || r).items
  const patch = (i: number, p: Partial<RoutineItem>) => setDraft({ ...d!, items: d!.items.map((x, j) => j === i ? { ...x, ...p } : x) })
  const move = (i: number, by: number) => { const a = [...d!.items]; const [x] = a.splice(i, 1); a.splice(i + by, 0, x); setDraft({ ...d!, items: a }) }
  const save = async () => {
    setSaving(true)
    try {
      const x = await api.put<Routine>(`${V1}/routines/${r.id}`, { name: d!.name, notes: d!.notes, outdoors: d!.outdoors, items: d!.items.map(i => ({ ...i, exercise: undefined })) })
      setR(x); setDraft(null); announceChange(); toast('Routine saved')
    } catch (e) { toast(errMsg(e), 'err') } finally { setSaving(false) }
  }
  const remove = async () => { await api.del(`${V1}/routines/${r.id}`); announceChange(); navigate('/routines') }
  const add = (list: Exercise[]) => {
    setPicking(false)
    setDraft({ ...d!, items: [...d!.items, ...list.map(e => ({ exercise_id: e.id, exercise: e, sets: 3, reps_min: e.kind === 'weight' ? 8 : null, reps_max: e.kind === 'weight' || e.kind === 'reps' ? (e.kind === 'weight' ? 12 : 10) : null, weight: null, duration_s: e.kind === 'time' ? 60 : null, distance_m: null, rest_s: null, notes: '' }))] })
  }
  return (
    <div className="stack loose">
      <div className="detail-head">
        <span className="detail-ico"><Icon name={(d || r).outdoors ? 'sun' : 'dumbbell'} size={22} /></span>
        <div className="grow col">
          {d ? <input className="title-in" value={d.name} onChange={e => setDraft({ ...d, name: e.target.value })} aria-label="Routine name" autoFocus /> : <h1 className="page-title">{r.name}</h1>}
          <p className="muted small">{r.items.length} exercises{r.last_done ? ` · last done ${fmtDate(r.last_done, false)}, ${r.times}× in all` : ''}</p>
        </div>
        <div className="row toolbar">
          {d ? <>
            <button type="button" className="btn" onClick={() => setDraft(null)}>Cancel</button>
            <button type="button" className="btn primary" disabled={saving || !d.name.trim()} onClick={save}>{saving ? <Spinner /> : <Icon name="check" size={15} />}Save</button>
          </> : <>
            <button type="button" className="btn" onClick={() => setDraft(r)}><Icon name="edit" size={15} />Edit</button>
            <button type="button" className="btn primary" disabled={busy != null || !r.items.length} onClick={() => start(r.id)}>{busy ? <Spinner /> : <Icon name="play" size={15} />}Start</button>
            <MenuButton label="More" align="right" items={[{ label: 'Delete this routine', icon: 'trash', danger: true, onSelect: () => setDel(true) }]}><Icon name="more" size={18} /></MenuButton>
          </>}
        </div>
      </div>
      {d ? <Card><div className="stack">
        <textarea rows={2} value={d.notes} placeholder="Notes: what this session is for" onChange={e => setDraft({ ...d, notes: e.target.value })} aria-label="Notes" />
        <label className="check-row"><Switch size="sm" checked={d.outdoors} onChange={v => setDraft({ ...d, outdoors: v })} label="Outdoors" /> Outdoors — show the day's weather from Pocket Weather</label>
      </div></Card> : r.notes ? <p className="notes">{r.notes}</p> : null}

      <Card title="Exercises" flush>
        {!items.length && <p className="muted pad">No exercises yet{d ? '' : '; Edit to add some'}.</p>}
        <ul className="list">{items.map((it, i) => {
          const ex = it.exercise
          if (!d) return (
            <li key={i}><Link to={`/exercises/${it.exercise_id}`} className="list-row">
              <span className="list-ico num-ico">{i + 1}</span>
              <span className="row-main grow"><span className="row-title">{ex?.name}</span><span className="muted small">{ex ? muscleLabel(ex.muscle) : ''}{it.rest_s ? ` · rest ${clock(it.rest_s)}` : ''}{it.notes ? ` · ${it.notes}` : ''}</span></span>
              <span className="row-amount tnum">{aim(it, ex, u)}</span>
            </Link></li>
          )
          const kind = ex?.kind || 'weight'
          return (
            <li key={i} className="list-row item-edit">
              <span className="list-ico num-ico">{i + 1}</span>
              <div className="grow col item-fields">
                <b className="ellipsis">{ex?.name}</b>
                <div className="row wrap">
                  <label className="mini">Sets<NumInput label="Sets" value={it.sets} onChange={v => patch(i, { sets: Math.max(1, Math.round(v || 1)) })} /></label>
                  {(kind === 'weight' || kind === 'reps') && <label className="mini">Reps<span className="row nowrap"><NumInput label="Fewest reps" value={it.reps_min} onChange={v => patch(i, { reps_min: v == null ? null : Math.round(v) })} placeholder="8" />–<NumInput label="Most reps" value={it.reps_max} onChange={v => patch(i, { reps_max: v == null ? null : Math.round(v) })} placeholder="12" /></span></label>}
                  {kind === 'weight' && <label className="mini">{u.w}<NumInput label={`Weight in ${u.w}`} value={wOut(it.weight, u)} onChange={v => patch(i, { weight: wIn(v, u) })} placeholder="last time" /></label>}
                  {kind === 'cardio' && <label className="mini">{u.d}<NumInput label={`Distance in ${u.d}`} value={dOut(it.distance_m, u)} onChange={v => patch(i, { distance_m: dIn(v, u) })} /></label>}
                  {(kind === 'time' || kind === 'cardio') && <label className="mini">Time<ClockInput label="Time" value={it.duration_s} onChange={v => patch(i, { duration_s: v })} /></label>}
                  <label className="mini">Rest<ClockInput label="Rest" value={it.rest_s} onChange={v => patch(i, { rest_s: v })} placeholder={clock(settings?.rest_s ?? 90)} /></label>
                </div>
                <input className="small" value={it.notes} onChange={e => patch(i, { notes: e.target.value })} placeholder="Note (grip, seat height…)" aria-label="Note" />
              </div>
              <div className="col item-tools">
                <button type="button" className="btn ghost icon-only sm" disabled={i === 0} aria-label="Move up" onClick={() => move(i, -1)}><Icon name="chevron-up" size={15} /></button>
                <button type="button" className="btn ghost icon-only sm" disabled={i === items.length - 1} aria-label="Move down" onClick={() => move(i, 1)}><Icon name="chevron-down" size={15} /></button>
                <button type="button" className="btn ghost icon-only sm" aria-label="Remove" onClick={() => setDraft({ ...d, items: d.items.filter((_, j) => j !== i) })}><Icon name="trash" size={14} /></button>
              </div>
            </li>
          )
        })}</ul>
        {d && <div className="pad"><button type="button" className="btn" onClick={() => setPicking(true)}><Icon name="plus" size={15} />Add exercises</button></div>}
      </Card>

      {!d && !!r.weekdays?.length && <p className="muted small">On your week: {r.weekdays.map(w => DAYS[w - 1]).join(', ')}. <Link to="/routines">Change the week</Link></p>}

      {!d && recent.length > 0 && <Card title="Last times" flush><ul className="list">{recent.map(w => (
        <li key={w.id}><Link to={`/workouts/${w.id}`} className="list-row">
          <span className="row-main grow"><span className="row-title">{fmtDate(w.date)}{w.records > 0 && <span className="pr-mini"><Icon name="trophy" size={12} />{w.records}</span>}</span>
            <span className="muted small">{mins(Math.round(w.duration_s / 60))} · {w.set_count} sets{w.volume > 0 ? ` · ${tonnage(w.volume, u)}` : ''}</span></span>
        </Link></li>))}</ul></Card>}
      {picking && <ExercisePicker onClose={() => setPicking(false)} onPick={add} />}
      {del && <Confirm title="Delete this routine?" message="Its workouts stay in your history. It comes off your week and your calendar." confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={remove} />}
    </div>
  )
}
