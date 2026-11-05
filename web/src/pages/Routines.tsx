// Routines and the week: which routine on which day, at what time; and a
// routine drafted by Pocket AI from a goal.
import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, EQUIPMENT, useData } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { PlanDay, Routine, RoutineItem } from '../lib/types'
import { useStart } from '../components/StartSheet'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Empty, errMsg, Field, Loading, Modal, Segmented, Spinner, useToast } from '../components/ui'

const DAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']

export default function Routines() {
  const { overview, reload } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { start, busy } = useStart()
  const [plan, setPlan] = useState<PlanDay[] | null>(null)
  const [aiOpen, setAIOpen] = useState(false)
  useEffect(() => { api.get<PlanDay[]>(`${V1}/plan`).then(setPlan) }, [overview])
  const routines = overview?.routines
  const create = async () => {
    try {
      const r = await api.post<Routine>(`${V1}/routines`, { name: 'New routine', items: [] })
      announceChange(); navigate(`/routines/${r.id}?edit=1`)
    } catch (e) { toast(errMsg(e), 'err') }
  }
  const savePlan = async (next: PlanDay[]) => {
    setPlan(next)
    try { setPlan(await api.put<PlanDay[]>(`${V1}/plan`, next.filter(p => p.routine_id))); reload() } catch (e) { toast(errMsg(e), 'err') }
  }
  if (!routines || !plan) return <Loading />
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Routines</h1><p className="muted">Your sessions, and the days you do them.</p></div>
        <button type="button" className="btn" onClick={() => setAIOpen(true)}><Icon name="sparkles" size={15} />Make me a plan</button>
        <button type="button" className="btn primary" onClick={create}><Icon name="plus" size={15} />New routine</button>
      </div>
      {!routines.length ? <Card><Empty title="No routines yet" icon="list">A routine is a session you repeat: its exercises with how many sets and reps to aim for. Make one, keep a finished workout as one (its ⋯ menu), or let Pocket AI draft a plan.</Empty></Card> : (
        <div className="routine-grid">
          {routines.map(r => (
            <div key={r.id} className="card routine-card">
              <Link to={`/routines/${r.id}`} className="rc-head">
                <span className="list-ico"><Icon name={r.outdoors ? 'sun' : 'dumbbell'} size={16} /></span>
                <span className="grow col"><b>{r.name}</b><span className="muted small">{r.items.length} exercises{r.weekdays?.length ? ` · ${r.weekdays.map(d => DAYS[d - 1].slice(0, 3)).join(', ')}` : ''}</span></span>
              </Link>
              <p className="rc-list muted small">{r.items.map(i => i.exercise?.name.replace(/ \(.*\)$/, '')).join(' · ') || 'No exercises yet'}</p>
              <div className="row rc-foot">
                <span className="muted small grow">{r.last_done ? `Last ${fmtDate(r.last_done, false)} · ${r.times}×` : 'Not done yet'}</span>
                <button type="button" className="btn sm primary" disabled={busy != null || !r.items.length} onClick={() => start(r.id)}>{busy === r.id ? <Spinner /> : <Icon name="play" size={14} />}Start</button>
              </div>
            </div>
          ))}
        </div>
      )}
      {routines.length > 0 && <PlanCard plan={plan} routines={routines} onChange={savePlan} />}
      {aiOpen && <AIPlan onClose={() => setAIOpen(false)} />}
    </div>
  )
}

function PlanCard({ plan, routines, onChange }: { plan: PlanDay[]; routines: Routine[]; onChange: (p: PlanDay[]) => void }) {
  const { settings } = useApp()
  const set = (wd: number, p: Partial<PlanDay>) => {
    const cur = plan.find(x => x.weekday === wd) || { weekday: wd, routine_id: 0, time: '' }
    onChange([...plan.filter(x => x.weekday !== wd), { ...cur, ...p }].sort((a, b) => a.weekday - b.weekday))
  }
  const cal = settings?.calendar_plan && settings.calendar_via
  return (
    <Card title="Your week" sub={<>The routine for each day; the others are rest days. {cal ? 'Planned sessions go on your calendar for the next two weeks.' : <>To have them on Pocket Calendar, switch it on in <Link to="/settings/apps">Settings → Connected apps</Link>.</>}</>} flush>
      <ul className="list plan-list">
        {DAYS.map((d, i) => {
          const p = plan.find(x => x.weekday === i + 1)
          return (
            <li key={d} className="list-row plan-row">
              <span className="plan-day">{d}</span>
              <div className="plan-pick grow"><Select value={p?.routine_id ? String(p.routine_id) : ''} onChange={v => set(i + 1, { routine_id: Number(v) || 0 })} placeholder="Rest day" clearLabel="Rest day" ariaLabel={`${d}'s routine`}
                options={routines.map(r => ({ value: String(r.id), label: r.name }))} /></div>
              <input type="time" className="plan-time" value={p?.time || ''} disabled={!p?.routine_id} aria-label={`${d}'s time`} onChange={e => set(i + 1, { time: e.target.value })} />
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

type Draft = { name: string; notes: string; weekdays: number[]; items: (RoutineItem & { exercise_id: number })[]; keep: boolean }

/** "Make me a plan": the goal, days and equipment → draft routines to keep. */
function AIPlan({ onClose }: { onClose: () => void }) {
  const { toast } = useToast()
  const { reload } = useData()
  const [goal, setGoal] = useState('')
  const [days, setDays] = useState(3)
  const [minutes, setMinutes] = useState(60)
  const [level, setLevel] = useState<'new' | 'some' | 'experienced'>('some')
  const [equip, setEquip] = useState<string[]>(['barbell', 'dumbbell', 'machine', 'cable', 'bodyweight'])
  const [busy, setBusy] = useState(false)
  const [drafts, setDrafts] = useState<Draft[] | null>(null)
  const [onWeek, setOnWeek] = useState(true)
  const ask = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ ok: boolean; error?: string; routines: Draft[] }>(`${V1}/ai/routine`, { goal, days, minutes, level, equipment: equip })
      if (!r.ok) { toast(r.error || 'That did not work', 'err'); return }
      setDrafts(r.routines.map(d => ({ ...d, keep: true })))
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  const keep = async () => {
    setBusy(true)
    try {
      const plan: PlanDay[] = onWeek ? await api.get<PlanDay[]>(`${V1}/plan`) : []
      for (const d of drafts!.filter(x => x.keep)) {
        const r = await api.post<Routine>(`${V1}/routines`, { name: d.name, notes: d.notes, source: 'ai', items: d.items.map(i => ({ ...i, exercise: undefined })) })
        if (onWeek) for (const wd of d.weekdays) { const at = plan.findIndex(p => p.weekday === wd); const row = { weekday: wd, routine_id: r.id, time: plan[at]?.time || '' }; if (at >= 0) plan[at] = row; else plan.push(row) }
      }
      if (onWeek) await api.put(`${V1}/plan`, plan)
      announceChange(); reload(); toast('Routines saved'); onClose()
    } catch (e) { toast(errMsg(e), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Make me a plan" onClose={onClose} size="lg" footer={drafts ? <>
      <label className="check-row grow"><input type="checkbox" checked={onWeek} onChange={e => setOnWeek(e.target.checked)} /> Put them on my week</label>
      <button type="button" className="btn" onClick={() => setDrafts(null)}>Back</button>
      <button type="button" className="btn primary" disabled={busy || !drafts.some(d => d.keep)} onClick={keep}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Keep {drafts.filter(d => d.keep).length}</button>
    </> : <>
      <span className="grow muted small">Pocket AI drafts it from your own exercises and recent training; nothing is saved until you keep it.</span>
      <button type="button" className="btn primary" disabled={busy} onClick={ask}>{busy ? <Spinner /> : <Icon name="sparkles" size={15} />}{busy ? 'Thinking…' : 'Draft it'}</button>
    </>}>
      {!drafts ? <div className="stack loose">
        <Field label="What do you want from it?"><textarea rows={2} value={goal} onChange={e => setGoal(e.target.value)} placeholder="Get stronger at the big lifts; a bit more muscle on my back and shoulders" autoFocus /></Field>
        <div className="grid three-grid">
          <Field label="Days a week" group><Segmented value={days} onChange={setDays} options={[2, 3, 4, 5].map(n => ({ value: n, label: String(n) }))} /></Field>
          <Field label="Minutes a session" group><Segmented value={minutes} onChange={setMinutes} options={[30, 45, 60, 90].map(n => ({ value: n, label: String(n) }))} /></Field>
          <Field label="Experience" group><Segmented value={level} onChange={setLevel} options={[{ value: 'new', label: 'New' }, { value: 'some', label: 'Some' }, { value: 'experienced', label: 'A lot' }]} /></Field>
        </div>
        <Field label="What you have" group><div className="chips">{EQUIPMENT.filter(e => e.id !== 'other').map(e => <button key={e.id} type="button" className={`chip ${equip.includes(e.id) ? 'on' : ''}`} onClick={() => setEquip(x => x.includes(e.id) ? x.filter(y => y !== e.id) : [...x, e.id])}>{e.label}</button>)}</div></Field>
      </div> : <div className="stack">
        {drafts.map((d, i) => (
          <div key={i} className={`card inset draft ${d.keep ? '' : 'is-archived'}`}><div className="card-body stack tight">
            <label className="check-row"><input type="checkbox" checked={d.keep} onChange={e => setDrafts(drafts.map((x, j) => j === i ? { ...x, keep: e.target.checked } : x))} /><b>{d.name}</b><span className="muted small">{d.weekdays.map(w => DAYS[w - 1]).join(', ')}</span></label>
            {d.notes && <p className="muted small">{d.notes}</p>}
            <ol className="draft-items">{d.items.map((it, k) => <li key={k}>{it.exercise?.name} — {it.sets} × {it.duration_s ? `${it.duration_s} s` : it.reps_min && it.reps_max && it.reps_min !== it.reps_max ? `${it.reps_min}–${it.reps_max}` : it.reps_max || it.reps_min || ''}{it.rest_s ? <span className="muted small"> · rest {it.rest_s} s</span> : null}</li>)}</ol>
          </div></div>
        ))}
      </div>}
    </Modal>
  )
}
