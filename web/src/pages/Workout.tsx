// One workout. While it runs: the exercises with last time's numbers, a
// tick per set, a rest timer, saved as you go. Once finished: what was done,
// the records it set and what the other apps made of it, editable.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, muscleLabel, useData } from '../lib/data'
import { fmtDate, fmtISOTime } from '../lib/format'
import type { Exercise, Finished, SetType, WSet, Workout, WorkoutFull } from '../lib/types'
import { clock, mins, plates, RECORD_LABEL, setText, tonnage, unitsOf } from '../lib/units'
import { useCrumb } from '../components/Layout'
import { ExercisePicker } from '../components/ExercisePicker'
import { Icon } from '../components/icons'
import { SetFields, setHeads } from '../components/Inputs'
import { Confirm, Empty, errMsg, Loading, MenuButton, Modal, Spinner, useToast } from '../components/ui'

const TYPES: SetType[] = ['normal', 'warmup', 'drop', 'failure']
const TYPE_MARK: Record<SetType, string> = { normal: '', warmup: 'W', drop: 'D', failure: 'F' }
const TYPE_NAME: Record<SetType, string> = { normal: 'Normal set', warmup: 'Warm-up (not counted)', drop: 'Drop set', failure: 'To failure' }

/** The positions (exercises) of a workout in order. */
function blocks(sets: WSet[]): { position: number; exercise_id: number; sets: { s: WSet; i: number }[] }[] {
  const out: { position: number; exercise_id: number; sets: { s: WSet; i: number }[] }[] = []
  sets.forEach((s, i) => {
    let b = out.find(x => x.position === s.position)
    if (!b) { b = { position: s.position, exercise_id: s.exercise_id, sets: [] }; out.push(b) }
    b.sets.push({ s, i })
  })
  return out.sort((a, b) => a.position - b.position)
}

/** Renumber positions 0..n in their current order. */
function normalize(sets: WSet[]): WSet[] {
  const order = [...new Set(sets.map(s => s.position))].sort((a, b) => a - b)
  return sets.map(s => ({ ...s, position: order.indexOf(s.position) }))
}

export default function WorkoutPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { settings } = useApp()
  const { overview, reload: reloadOverview } = useData()
  const u = unitsOf(settings)
  const [full, setFull] = useState<WorkoutFull | null>(null)
  const [err, setErr] = useState('')
  const [sets, setSets] = useState<WSet[]>([])
  const [name, setName] = useState('')
  const [notes, setNotes] = useState('')
  const [editing, setEditing] = useState(false)
  const [saveState, setSaveState] = useState<'' | 'saving' | 'saved' | 'error'>('')
  const [picking, setPicking] = useState<null | { replace?: number }>(null)
  const [finishing, setFinishing] = useState(false)
  const [done, setDone] = useState<Finished | null>(null)
  const [confirm, setConfirm] = useState<'' | 'discard' | 'delete'>('')
  const [rest, setRest] = useState<{ until: number; total: number } | null>(null)
  const dirty = useRef(0)
  const [tick, setTick] = useState(0)

  const load = useCallback(() => {
    api.get<WorkoutFull>(`${V1}/workouts/${id}`).then(f => {
      setFull(f); setSets(f.workout.sets || []); setName(f.workout.name); setNotes(f.workout.notes); setErr('')
    }).catch(e => setErr(e instanceof ApiError && e.status === 404 ? 'notfound' : errMsg(e)))
  }, [id])
  useEffect(() => { load(); setEditing(false); setDone(null) }, [load])
  const w = full?.workout
  const running = !!w && !w.ended_at
  const live = running || editing
  useCrumb(w?.name, 'workouts')

  // exercises this page knows: the workout's own plus the person's list
  const { byId } = useData()
  const exOf = (eid: number): Exercise | undefined => full?.exercises[String(eid)] || byId[eid]

  // save a running workout as it changes (once things are quiet for a moment)
  const save = useCallback(async (body: Partial<{ name: string; notes: string; sets: WSet[]; feel: number }>) => {
    if (!w) return
    setSaveState('saving')
    try {
      const f = await api.put<WorkoutFull>(`${V1}/workouts/${w.id}`, body)
      setFull(prev => prev ? { ...prev, exercises: { ...prev.exercises, ...f.exercises }, previous: { ...prev.previous, ...f.previous }, workout: { ...f.workout } } : f)
      setSaveState('saved')
    } catch (e) { setSaveState('error'); toast(errMsg(e), 'err') }
  }, [w, toast])
  useEffect(() => {
    if (!running || !dirty.current) return
    const t = window.setTimeout(() => save({ name, notes, sets }), 700)
    return () => window.clearTimeout(t)
  }, [sets, name, notes, running]) // eslint-disable-line react-hooks/exhaustive-deps
  const change = (next: WSet[]) => { dirty.current++; setSets(next) }

  // the clock and the rest timer
  useEffect(() => {
    if (!running) return
    const t = window.setInterval(() => setTick(x => x + 1), 1000)
    return () => window.clearInterval(t)
  }, [running])
  const restLeft = rest ? Math.max(0, Math.round((rest.until - Date.now()) / 1000)) : 0
  useEffect(() => {
    if (rest && restLeft === 0) {
      try { navigator.vibrate?.([180, 80, 180]) } catch { /* not on this device */ }
      setRest(null)
    }
  }, [restLeft, rest])
  void tick

  const bl = useMemo(() => blocks(sets), [sets])

  const setPatch = (i: number, p: Partial<WSet>) => change(sets.map((s, j) => j === i ? { ...s, ...p } : s))
  const toggleDone = (i: number) => {
    const s = sets[i]
    const now = !s.done
    const filled = { ...s }
    const ex = exOf(s.exercise_id)
    // ticking an empty set takes last time's (or the set before's) numbers
    if (now && filled.weight == null && filled.reps == null && filled.duration_s == null && filled.distance_m == null) {
      const before = sets.slice(0, i).reverse().find(x => x.position === s.position)
      const prev = full?.previous[String(s.exercise_id)]?.[s.set_no ? s.set_no - 1 : 0]
      const src = before || prev
      if (src) Object.assign(filled, { weight: src.weight, reps: src.reps, duration_s: src.duration_s, distance_m: src.distance_m })
    }
    filled.done = now
    change(sets.map((x, j) => j === i ? filled : x))
    if (now && running && s.type !== 'warmup') {
      const r = s.rest_s ?? settings?.rest_s ?? 90
      if (r > 0 && ex?.kind !== 'cardio') setRest({ until: Date.now() + r * 1000, total: r })
    }
  }
  const addSet = (position: number) => {
    const same = sets.filter(s => s.position === position)
    const last = same[same.length - 1]
    const ns: WSet = { exercise_id: last.exercise_id, position, type: 'normal', weight: last.weight, reps: last.reps, duration_s: last.duration_s, distance_m: last.distance_m, rpe: null, done: false, note: '', rest_s: last.rest_s }
    const at = sets.lastIndexOf(last) + 1
    change([...sets.slice(0, at), ns, ...sets.slice(at)])
  }
  const removeSet = (i: number) => change(normalize(sets.filter((_, j) => j !== i)))
  const removeExercise = (position: number) => change(normalize(sets.filter(s => s.position !== position)))
  const move = (position: number, d: -1 | 1) => {
    const other = position + d
    change(normalize(sets.map(s => ({ ...s, position: s.position === position ? other : s.position === other ? position : s.position }))))
  }
  const addExercises = (list: Exercise[]) => {
    const target = picking?.replace
    setPicking(null)
    if (target != null) {
      const e = list[0]
      change(sets.map(s => s.position === target ? { ...s, exercise_id: e.id, weight: null, reps: null, duration_s: null, distance_m: null, done: false } : s))
      return
    }
    let pos = sets.reduce((m, s) => Math.max(m, s.position + 1), 0)
    const add: WSet[] = []
    for (const e of list) {
      const prev = full?.previous[String(e.id)] || []
      const n = Math.max(1, Math.min(prev.filter(p => p.type !== 'warmup').length || 3, 6))
      for (let k = 0; k < n; k++) add.push({ exercise_id: e.id, position: pos, type: 'normal', weight: null, reps: null, duration_s: null, distance_m: null, rpe: null, done: false, note: '', rest_s: null })
      pos++
    }
    change([...sets, ...add])
  }

  const finish = async () => {
    if (!w) return
    if (!sets.some(s => s.done)) { toast('Tick the sets you did first (the round button on each set)', 'err'); return }
    setFinishing(true)
    try {
      await api.put(`${V1}/workouts/${w.id}`, { name, notes, sets })
      const r = await api.post<Finished>(`${V1}/workouts/${w.id}/finish`, {})
      setDone(r); setRest(null)
      setFull(r); setSets(r.workout.sets || []); dirty.current = 0
      announceChange()
    } catch (e) { toast(errMsg(e), 'err') } finally { setFinishing(false) }
  }
  const discard = async () => {
    if (!w) return
    await api.del(`${V1}/workouts/${w.id}`).catch(() => {})
    announceChange(); setConfirm('')
    navigate(running ? '/' : '/history')
  }
  const saveEdit = async () => {
    if (!w) return
    try {
      const f = await api.put<WorkoutFull>(`${V1}/workouts/${w.id}`, { name, notes, sets })
      setFull(f); setSets(f.workout.sets || []); setEditing(false); announceChange(); toast('Saved')
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const keepAsRoutine = async () => {
    if (!w) return
    try {
      const r = await api.post<{ id: number }>(`${V1}/workouts/${w.id}/routine`, { name: w.name })
      announceChange(); toast('Kept as a routine'); navigate(`/routines/${r.id}`)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const resend = async () => {
    if (!w) return
    try {
      const r = await api.post<{ links: Finished['links'] }>(`${V1}/workouts/${w.id}/resend`)
      load(); toast(linkSummary(r.links) || 'Sent again')
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const setFeel = (f: number) => { if (!w) return; setFull({ ...full!, workout: { ...w, feel: f } }); save({ feel: f }) }

  // keyboard: space ticks the next set, ⌘↵ finishes
  useEffect(() => {
    if (!running) return
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (t.closest('input, textarea, [contenteditable="true"], .modal, .menu')) return
      if (e.key === ' ') { const i = sets.findIndex(s => !s.done); if (i >= 0) { e.preventDefault(); toggleDone(i) } }
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); finish() }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  if (err === 'notfound') return <Empty title="No such workout" icon="dumbbell"><Link to="/history">See your workouts</Link></Empty>
  if (err) return <div className="callout error"><Icon name="alert" size={15} />{err}</div>
  if (!full || !w) return <Loading />

  const started = new Date(w.started_at)
  const elapsed = running ? Math.floor((Date.now() - started.getTime()) / 1000) : w.duration_s
  const doneSets = sets.filter(s => s.done && s.type !== 'warmup')
  const volume = doneSets.reduce((t, s) => t + (s.weight != null && s.reps != null ? s.weight * s.reps : 0), 0)
  const weather = running && full.routine?.outdoors && overview?.today?.routine.id === full.routine.id ? overview?.today?.weather : undefined
  const playlist = running ? overview?.playlist : null

  return (
    <div className={`workout-page stack loose ${running ? 'is-live' : ''}`}>
      <div className="detail-head">
        <span className="detail-ico"><Icon name={running ? 'timer' : 'dumbbell'} size={22} /></span>
        <div className="grow col">
          {live
            ? <input className="title-in" value={name} onChange={e => { dirty.current++; setName(e.target.value) }} aria-label="Workout name" />
            : <h1 className="page-title">{w.name}</h1>}
          <p className="muted small">
            {fmtDate(w.date)} · {fmtISOTime(w.started_at)}
            {full.routine && <> · <Link to={`/routines/${full.routine.id}`}>{full.routine.name}</Link></>}
            {w.source && w.source !== 'api' && <> · from {sourceName(w.source)}</>}
          </p>
        </div>
        {running
          ? <div className="live-stats">
              <span className="live-clock tnum" title="Time since you started">{clock(elapsed)}</span>
              <span className={`save-state small ${saveState}`} aria-live="polite">{saveState === 'saving' ? 'Saving…' : saveState === 'error' ? 'Not saved' : saveState === 'saved' ? 'Saved' : ''}</span>
            </div>
          : <div className="row toolbar">
              {editing
                ? <><button type="button" className="btn" onClick={() => { setEditing(false); setSets(w.sets || []); setName(w.name); setNotes(w.notes) }}>Cancel</button><button type="button" className="btn primary" onClick={saveEdit}><Icon name="check" size={15} />Save</button></>
                : <button type="button" className="btn" onClick={() => setEditing(true)}><Icon name="edit" size={15} />Edit</button>}
              <MenuButton label="More" align="right" items={[
                { label: 'Keep as a routine', icon: 'list-plus', onSelect: keepAsRoutine },
                { label: 'Send to Pocket Health and Habits again', icon: 'refresh', onSelect: resend },
                'sep',
                { label: 'Delete this workout', icon: 'trash', danger: true, onSelect: () => setConfirm('delete') },
              ]}><Icon name="more" size={18} /></MenuButton>
            </div>}
      </div>

      {!running && <div className="stats-row tight">
        <div className="stat"><span className="stat-l">Time</span><span className="stat-v">{mins(Math.round(w.duration_s / 60))}</span></div>
        <div className="stat"><span className="stat-l">Sets</span><span className="stat-v">{w.set_count}</span></div>
        {w.volume > 0 && <div className="stat"><span className="stat-l">Lifted</span><span className="stat-v">{tonnage(w.volume, u)}</span></div>}
        {w.distance_m > 0 && <div className="stat"><span className="stat-l">Distance</span><span className="stat-v">{setText({ distance_m: w.distance_m, weight: null, reps: null, duration_s: null }, { kind: 'cardio' } as Exercise, u)}</span></div>}
        <div className="stat"><span className="stat-l">Records</span><span className="stat-v">{w.records ? <><Icon name="trophy" size={15} className="gold" /> {w.records}</> : '–'}</span></div>
      </div>}

      {!running && !editing && <LinkLine w={w} settings={settings} />}

      {(weather || playlist) && <div className="row wrap live-extras">
        {weather && <div className="callout info grow"><Icon name="sun" size={15} /><div><b>Weather in {weather.place}:</b> {weather.text}</div></div>}
        {playlist && <a className="btn" href={playlist.url} target="_blank" rel="noreferrer"><Icon name="music" size={15} />{playlist.name}</a>}
      </div>}

      {bl.length === 0 && <Empty title="No exercises yet" icon="dumbbell">Add the exercises you are doing; each gets its sets with last time's numbers.</Empty>}

      {bl.map((b, bi) => {
        const ex = exOf(b.exercise_id)
        const prev = full.previous[String(b.exercise_id)] || []
        const [h1, h2] = setHeads(ex, u)
        const prevWork = prev.filter(p => p.type !== 'warmup')
        let workNo = 0
        return (
          <section key={b.position} className="card ex-block">
            <div className="ex-head">
              <div className="grow col">
                <Link to={`/exercises/${b.exercise_id}`} className="ex-name">{ex?.name || 'Exercise'}</Link>
                <span className="muted small">{ex ? muscleLabel(ex.muscle) : ''}{prevWork.length > 0 && live ? <> · last time {prevWork.map(p => setText(p, ex, u)).join(', ')}</> : ''}</span>
              </div>
              {live && <MenuButton label={`${ex?.name} options`} align="right" items={[
                { label: 'Swap for another exercise', icon: 'repeat', onSelect: () => setPicking({ replace: b.position }) },
                { label: 'Move up', icon: 'arrow-up', disabled: bi === 0, onSelect: () => move(b.position, -1) },
                { label: 'Move down', icon: 'arrow-down', disabled: bi === bl.length - 1, onSelect: () => move(b.position, 1) },
                'sep',
                { label: 'Remove this exercise', icon: 'trash', danger: true, onSelect: () => removeExercise(b.position) },
              ]}><Icon name="more" size={17} /></MenuButton>}
            </div>
            <div className={`set-grid ${live ? 'live' : 'read'}`}>
              {live && <><span className="set-h">Set</span><span className="set-h hide-xs">Last time</span><span className="set-h">{h1}</span><span className="set-h">{h2}</span><span /></>}
              {b.sets.map(({ s, i }) => {
                const no = s.type === 'warmup' ? 0 : ++workNo
                const p = s.type === 'warmup' ? prev.filter(x => x.type === 'warmup')[b.sets.filter(x => x.s.type === 'warmup').findIndex(x => x.i === i)] : prevWork[no - 1]
                if (!live) return (
                  <div key={i} className={`set-row read ${s.type}`}>
                    <span className={`set-no t-${s.type}`} title={TYPE_NAME[s.type]}>{TYPE_MARK[s.type] || no}</span>
                    <span className="set-val">{setText(s, ex, u)}{s.rpe ? <span className="muted small"> · RPE {s.rpe}</span> : ''}</span>
                    <span className="set-pr">{s.pr?.length ? <span className="pr-badge" title={s.pr.map(k => RECORD_LABEL[k]).join(', ')}><Icon name="trophy" size={12} />{RECORD_LABEL[s.pr[0]]}</span> : null}</span>
                  </div>
                )
                return (
                  <div key={i} className={`set-row live ${s.done ? 'is-done' : ''} ${s.type}`}>
                    <MenuButton label={`Set ${no || 'warm-up'}`} className={`set-no btn-no t-${s.type}`} items={[
                      ...TYPES.map(t => ({ label: TYPE_NAME[t], checked: s.type === t, onSelect: () => setPatch(i, { type: t }) })),
                      'sep' as const,
                      { label: 'Remove this set', icon: 'trash', danger: true, onSelect: () => removeSet(i) },
                    ]}>{TYPE_MARK[s.type] || no}</MenuButton>
                    <span className="set-prev hide-xs muted small tnum">{p ? setText(p, ex, u) : '–'}</span>
                    <SetFields set={s} ex={ex} u={u} onChange={x => setPatch(i, x)} onEnter={() => { if (!s.done) toggleDone(i) }} />
                    {running
                      ? <button type="button" className={`set-check ${s.done ? 'on' : ''}`} aria-pressed={s.done} aria-label={s.done ? 'Done; tap to undo' : 'Mark this set done'} onClick={() => toggleDone(i)}><Icon name="check" size={16} strokeWidth={2.6} /></button>
                      : <button type="button" className="btn ghost icon-only sm" aria-label="Remove this set" onClick={() => removeSet(i)}><Icon name="x" size={14} /></button>}
                  </div>
                )
              })}
            </div>
            {live && <div className="ex-foot row">
              <button type="button" className="link-btn small" onClick={() => addSet(b.position)}>+ Add a set</button>
              {settings?.plates && ex?.equipment === 'barbell' && ex.kind === 'weight' && (() => {
                const next = b.sets.find(x => !x.s.done && x.s.weight)?.s.weight
                return next ? <span className="muted small grow right">Each side: {plates(next, settings.bar_kg || 20, u)}</span> : null
              })()}
            </div>}
          </section>
        )
      })}

      {live && <button type="button" className="btn self-start" onClick={() => setPicking({})}><Icon name="plus" size={15} />Add exercises</button>}

      {(live || notes) && <div className="card"><div className="card-body">
        {live ? <textarea rows={2} value={notes} placeholder="Notes: how it went, what to change next time" onChange={e => { dirty.current++; setNotes(e.target.value) }} aria-label="Notes" />
          : <p className="notes">{notes}</p>}
      </div></div>}

      {!running && !editing && <div className="row feel-row"><span className="muted small">How did it feel?</span><Feel value={w.feel} onChange={setFeel} /></div>}

      {running && <div className="live-bar">
        {rest
          ? <div className="rest">
              <span className="rest-bar" style={{ ['--p' as string]: `${(1 - restLeft / rest.total) * 100}%` }} />
              <Icon name="timer" size={16} /><span className="tnum rest-left">Rest {clock(restLeft)}</span>
              <button type="button" className="btn sm" onClick={() => setRest({ ...rest, until: Math.max(Date.now(), rest.until - 15000) })}>−15 s</button>
              <button type="button" className="btn sm" onClick={() => setRest({ until: rest.until + 15000, total: rest.total + 15 })}>+15 s</button>
              <button type="button" className="btn sm ghost" onClick={() => setRest(null)}>Skip</button>
            </div>
          : <span className="muted small grow live-sum tnum">{doneSets.length} {doneSets.length === 1 ? 'set' : 'sets'} done{volume > 0 ? ` · ${tonnage(volume, u)}` : ''}</span>}
        <button type="button" className="btn ghost danger-text" onClick={() => setConfirm('discard')}>Discard</button>
        <button type="button" className="btn primary" disabled={finishing} onClick={finish}>{finishing ? <Spinner /> : <Icon name="check" size={15} />}Finish</button>
      </div>}

      {picking && <ExercisePicker multi={picking.replace == null} title={picking.replace == null ? 'Add exercises' : 'Swap for'} onClose={() => setPicking(null)} onPick={addExercises} />}
      {confirm === 'discard' && <Confirm title="Discard this workout?" message="Its sets are not kept anywhere." confirmLabel="Discard" onCancel={() => setConfirm('')} onConfirm={discard} />}
      {confirm === 'delete' && <Confirm title="Delete this workout?" message="Its sets, and the records it set, go with it. A copy already sent to Pocket Health stays there." confirmLabel="Delete" onCancel={() => setConfirm('')} onConfirm={discard} />}
      {done && <FinishSummary r={done} onClose={() => { setDone(null); reloadOverview() }} />}
    </div>
  )
}

function sourceName(s: string): string {
  return ({ strong: 'Strong', hevy: 'Hevy', import: 'an import', ai: 'Pocket AI', voice: 'Pocket Voice', ai_mcp: 'Pocket AI' } as Record<string, string>)[s] || `Pocket ${s[0].toUpperCase()}${s.slice(1)}`
}

function linkSummary(links: Finished['links']): string {
  const parts: string[] = []
  if (links.health?.state === 'done') parts.push('in Pocket Health')
  if (links.habit?.state === 'done') parts.push(`“${links.habit.detail}” ticked in Pocket Habits`)
  return parts.length ? 'Saved ' + parts.join(', ') : ''
}

/** What Pocket Health and Pocket Habits made of a finished workout. */
function LinkLine({ w, settings }: { w: Workout; settings: ReturnType<typeof useApp>['settings'] }) {
  const items: React.ReactNode[] = []
  if (w.health_ref) items.push(<span key="h" className="link-state ok"><Icon name="heart-pulse" size={14} />In Pocket Health</span>)
  else if (w.health_error) items.push(<span key="h" className="link-state bad" title={w.health_error}><Icon name="alert" size={14} />Not in Pocket Health yet</span>)
  if (w.habit_done) items.push(<span key="b" className="link-state ok"><Icon name="check-circle" size={14} />{settings?.habit_name || 'Habit'} ticked</span>)
  if (!items.length) return null
  return <div className="row wrap link-line">{items}</div>
}

function Feel({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const words = ['', 'Hard going', 'Tough', 'OK', 'Good', 'Great']
  return (
    <div className="feel" role="radiogroup" aria-label="How it felt">
      {[1, 2, 3, 4, 5].map(n => <button key={n} type="button" role="radio" aria-checked={value === n} className={value === n ? 'on' : ''} title={words[n]} onClick={() => onChange(n)}>{words[n]}</button>)}
    </div>
  )
}

/** After Finish: the records, the week, and the other apps. */
function FinishSummary({ r, onClose }: { r: Finished; onClose: () => void }) {
  const { settings } = useApp()
  const u = unitsOf(settings)
  const w = r.workout
  const link = (k: string, label: string) => {
    const l = r.links[k]
    if (!l || l.state === 'off') return null
    return <li className={`link-state ${l.state === 'done' ? 'ok' : 'bad'}`}><Icon name={l.state === 'done' ? 'check-circle' : 'alert'} size={14} />{l.state === 'done' ? label : `${label}: ${l.detail}`}</li>
  }
  return (
    <Modal title="Workout done" onClose={onClose} size="sm" footer={<button type="button" className="btn primary" onClick={onClose} autoFocus>Close</button>}>
      <div className="stack finish">
        <p className="finish-line"><b>{w.name}</b>: {mins(Math.round(w.duration_s / 60))}, {w.set_count} sets{w.volume > 0 ? `, ${tonnage(w.volume, u)} lifted` : ''}.</p>
        {r.records.length > 0 && <div className="records-won">
          <h3><Icon name="trophy" size={16} className="gold" />{r.records.length === 1 ? 'A new record' : `${r.records.length} new records`}</h3>
          <ul className="plain">{r.records.map((x, i) => <li key={i}>{x.text.replace(/(\d+(\.\d+)?) kg/g, (_, n) => u.w === 'lb' ? `${Math.round(Number(n) * 2.20462)} lb` : `${n} kg`)}</li>)}</ul>
        </div>}
        {r.week.goal > 0 && <p className={r.week.met ? 'ok' : ''}>{r.week.met ? <><Icon name="check-circle" size={15} /> Weekly goal met: {r.week.done} of {r.week.goal}.</> : <>This week: {r.week.done} of {r.week.goal} workouts.</>}</p>}
        <ul className="plain stack tight">{link('health', 'Recorded in Pocket Health')}{link('habit', `${r.links.habit?.detail || 'Your habit'} ticked in Pocket Habits`)}</ul>
      </div>
    </Modal>
  )
}
