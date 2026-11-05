// Today: the workout in progress or today's plan, the week against the
// goal (with the runs from Pocket Health), recent workouts and records, and
// the week in plain words from Pocket AI.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { kindIcon, useData } from '../lib/data'
import { fmtClock, fmtDate, todayISO, weekday } from '../lib/format'
import type { Week } from '../lib/types'
import { distance, mins, tonnage, unitsOf } from '../lib/units'
import { openLog, openStart } from '../components/Layout'
import { useStart } from '../components/StartSheet'
import { Icon } from '../components/icons'
import { Card, Empty, Loading, Spinner } from '../components/ui'

export default function Today() {
  const { overview, error } = useData()
  const { settings, user } = useApp()
  const u = unitsOf(settings)
  const { start, busy } = useStart()
  if (error && !overview) return <div className="callout error"><Icon name="alert" size={15} />{error}</div>
  if (!overview) return <Loading />
  const { active, today, week, recent, records } = overview
  const first = !recent.length && !overview.routines.length && !active
  const hello = new Date().getHours() < 12 ? 'Good morning' : new Date().getHours() < 18 ? 'Good afternoon' : 'Good evening'
  return (
    <div className="stack loose today">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{hello}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</h1>
          <p className="muted">{fmtDate(todayISO())}{overview.streak > 1 ? ` · ${overview.streak} weeks in a row on goal` : ''}</p></div>
        <button type="button" className="btn" onClick={openLog}><Icon name="pen" size={15} />Log a workout</button>
      </div>

      {active ? (
        <Link to={`/workouts/${active.id}`} className="hero-card live">
          <span className="hero-ico"><Icon name="timer" size={22} /></span>
          <span className="grow"><span className="hero-k">In progress</span><b className="hero-t">{active.name}</b><span className="muted small">Started {fmtClock(new Date(active.started_at).toTimeString().slice(0, 5))}</span></span>
          <span className="btn primary"><Icon name="play" size={15} />Go back to it</span>
        </Link>
      ) : today ? (
        <div className={`hero-card ${today.done ? 'done' : ''}`}>
          <span className="hero-ico"><Icon name={today.done ? 'check-circle' : today.routine.outdoors ? 'sun' : 'calendar-check'} size={22} /></span>
          <span className="grow col">
            <span className="hero-k">{today.done ? 'Done today' : `Today${today.time ? ` at ${fmtClock(today.time)}` : ''}`}</span>
            <Link to={`/routines/${today.routine.id}`} className="hero-t">{today.routine.name}</Link>
            <span className="muted small">{today.routine.items.length} exercises · about {mins(today.minutes)}{today.routine.items.length ? ': ' + today.routine.items.slice(0, 4).map(i => i.exercise?.name.replace(/ \(.*\)$/, '')).join(', ') + (today.routine.items.length > 4 ? '…' : '') : ''}</span>
            {today.weather && <span className="hero-weather small"><Icon name="sun" size={14} />{today.weather.text}</span>}
          </span>
          {!today.done && <button type="button" className="btn primary" disabled={busy != null} onClick={() => start(today.routine.id)}>{busy ? <Spinner /> : <Icon name="play" size={15} />}Start</button>}
        </div>
      ) : (
        <div className="hero-card rest">
          <span className="hero-ico"><Icon name="bed" size={22} /></span>
          <span className="grow col"><span className="hero-k">Today</span><b className="hero-t">{overview.routines.length ? 'A rest day' : 'Nothing planned'}</b>
            <span className="muted small">{overview.routines.length ? 'Nothing on your plan for today. Train anyway, or rest.' : 'Start an empty workout, or make a routine for the days you train.'}</span></span>
          <button type="button" className="btn primary" onClick={openStart}><Icon name="plus" size={15} />Start a workout</button>
        </div>
      )}

      {first && <Card><Empty title="Your training log" icon="dumbbell">
        Start a workout and tick off your sets as you go, or bring your history from Strong or Hevy (<Link to="/settings/data">Settings → Backup & data</Link>).
        Make a routine for each kind of session and put it on your week in <Link to="/routines">Routines</Link>; Pocket AI can draft them for you.
      </Empty></Card>}

      <WeekCard week={week} />

      <div className="two-grid">
        <Card title="Recent workouts" actions={<Link to="/history" className="small">All</Link>} flush>
          {recent.length ? <ul className="list">{recent.map(w => (
            <li key={w.id}><Link to={`/workouts/${w.id}`} className="list-row">
              <span className="list-ico"><Icon name="dumbbell" size={16} /></span>
              <span className="row-main grow"><span className="row-title">{w.name}{w.records > 0 && <span className="pr-mini" title={`${w.records} records`}><Icon name="trophy" size={12} />{w.records}</span>}</span>
                <span className="muted small">{fmtDate(w.date, false)} · {mins(Math.round(w.duration_s / 60))} · {w.set_count} sets{w.volume > 0 ? ` · ${tonnage(w.volume, u)}` : ''}{w.distance_m > 0 ? ` · ${distance(w.distance_m, u)}` : ''}</span></span>
            </Link></li>))}</ul> : <p className="muted pad">No workouts yet.</p>}
        </Card>
        <Card title="New records" actions={<Link to="/progress" className="small">Progress</Link>} flush>
          {records.length ? <ul className="list">{records.map((r, i) => (
            <li key={i}><Link to={`/exercises/${r.exercise_id}`} className="list-row">
              <span className="list-ico gold-bg"><Icon name="trophy" size={16} /></span>
              <span className="row-main grow"><span className="row-title">{r.exercise}</span><span className="muted small">{r.text.replace(`${r.exercise}: `, '').replace(/(\d+(?:\.\d+)?) kg/g, (_, n) => u.w === 'lb' ? `${Math.round(Number(n) * 2.20462)} lb` : `${n} kg`)} · {r.date ? fmtDate(r.date, false) : ''}</span></span>
            </Link></li>))}</ul> : <p className="muted pad">Records show up here when you beat your best at an exercise.</p>}
        </Card>
      </div>

      {settings?.ai_review && recent.length > 0 && <AIReview />}
    </div>
  )
}

/** The week: a column per day with its workouts and what was planned. */
export function WeekCard({ week }: { week: Week }) {
  const { settings } = useApp()
  const u = unitsOf(settings)
  const today = todayISO()
  const pct = week.goal ? Math.min(100, Math.round(week.done / week.goal * 100)) : 0
  return (
    <Card title="This week" sub={week.goal ? `${week.done} of ${week.goal} workouts${week.done >= week.goal ? ' — goal met' : ''}${week.health ? ' · runs and rides from Pocket Health count too' : ''}` : `${week.done} workouts`}
      actions={week.goal ? <span className="goal-ring" style={{ ['--p' as string]: `${pct}%` }} title={`${pct}% of your goal`}><span>{week.done}/{week.goal}</span></span> : undefined}>
      <div className="week-grid">
        {week.days.map(d => (
          <div key={d.date} className={`week-day ${d.date === today ? 'is-today' : ''} ${d.items.length ? 'has' : ''}`}>
            <span className="wd-name">{weekday(d.date)}</span>
            <span className="wd-num">{Number(d.date.slice(8))}</span>
            <div className="wd-items">
              {d.items.map(it => it.health
                ? <span key={`h${it.id}`} className="wd-item health" title={`${it.name} (Pocket Health)`}><Icon name={kindIcon(it.kind)} size={13} /><span className="ellipsis">{it.distance_m ? distance(it.distance_m, u) : `${it.minutes} min`}</span></span>
                : <Link key={it.id} to={`/workouts/${it.id}`} className="wd-item" title={it.name}><Icon name="dumbbell" size={13} /><span className="ellipsis">{it.name}</span></Link>)}
              {!d.items.length && d.planned && <span className={`wd-item planned ${d.date < today ? 'missed' : ''}`} title={d.date < today ? 'Planned, not done' : 'Planned'}><Icon name="calendar" size={13} /><span className="ellipsis">{d.planned.routine}</span></span>}
            </div>
          </div>
        ))}
      </div>
    </Card>
  )
}

/** The week (or month) in plain words, written by the AI and kept for the day. */
export function AIReview({ period = 'week' }: { period?: 'week' | 'month' }) {
  const [r, setR] = useState<{ ok: boolean; text?: string; error?: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const run = (fresh: boolean) => {
    setBusy(true)
    api.post<{ ok: boolean; text?: string; error?: string }>(`${V1}/ai/review`, { period, fresh }).then(setR).catch(e => setR({ ok: false, error: (e as Error).message })).finally(() => setBusy(false))
  }
  useEffect(() => { run(false) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Card title={<><Icon name="sparkles" size={16} className="accent-ico" /> {period === 'week' ? 'Your training lately' : 'The last four weeks'}</>}
      actions={<button type="button" className="btn ghost sm" disabled={busy} onClick={() => run(true)} title="Write it again"><Icon name="refresh" size={14} /></button>}>
      {!r ? <div className="ai-wait muted small"><Spinner /> Reading your log…</div>
        : r.ok ? <p className="ai-text">{r.text}</p>
          : <p className="muted small">{r.error || 'Not available'} {/Pocket AI|key/i.test(r.error || '') && <Link to="/settings/ai">Settings → AI</Link>}</p>}
    </Card>
  )
}
