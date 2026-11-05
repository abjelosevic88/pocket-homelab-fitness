// Progress: workouts per week against the goal, sets per muscle, your
// weight from Pocket Health, the latest records and the totals.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { muscleLabel, useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { NewRecord } from '../lib/types'
import { mins, tonnage, unitsOf, weight } from '../lib/units'
import { Bars, LineChart } from '../components/Charts'
import { Icon } from '../components/icons'
import { Card, Loading, Segmented } from '../components/ui'
import { AIReview } from './Today'

interface Progress {
  weeks: { start: string; workouts: number; health: number; volume: number; sets: number; minutes: number }[]
  goal: number; muscles: { muscle: string; sets: number }[]; streak: number; health: boolean
  totals: { workouts: number; hours: number; volume: number; since: string }
  records: NewRecord[]; weight: { date: string; value: number }[] | null
}

export default function ProgressPage() {
  const { settings } = useApp()
  const u = unitsOf(settings)
  const [weeks, setWeeks] = useState(12)
  const [p, setP] = useState<Progress | null>(null)
  const load = () => { api.get<Progress>(`${V1}/progress?weeks=${weeks}`).then(setP) }
  useEffect(load, [weeks]) // eslint-disable-line react-hooks/exhaustive-deps
  useOnChange(load)
  if (!p) return <Loading />
  const maxM = Math.max(1, ...p.muscles.map(m => m.sets))
  const lastW = p.weeks[p.weeks.length - 1]
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Progress</h1><p className="muted">{p.totals.workouts ? `${p.totals.workouts} workouts since ${fmtDate(p.totals.since)}` : 'Nothing logged yet'}{p.streak > 1 ? ` · ${p.streak} weeks in a row on goal` : ''}</p></div>
        <Segmented size="sm" value={weeks} onChange={setWeeks} options={[{ value: 12, label: '12 weeks' }, { value: 26, label: '6 months' }, { value: 52, label: 'A year' }]} />
      </div>
      <div className="stats-row">
        <div className="stat"><span className="stat-l">This week</span><span className="stat-v">{lastW ? lastW.workouts + lastW.health : 0}{p.goal ? ` of ${p.goal}` : ''}</span><span className="stat-s small">{lastW ? mins(lastW.minutes) : ''}</span></div>
        <div className="stat"><span className="stat-l">Workouts</span><span className="stat-v">{p.totals.workouts}</span><span className="stat-s small">{p.totals.hours} hours</span></div>
        <div className="stat"><span className="stat-l">Lifted in all</span><span className="stat-v">{tonnage(p.totals.volume, u)}</span></div>
        {p.weight && p.weight.length > 0 && <div className="stat"><span className="stat-l">Your weight</span><span className="stat-v">{weight(p.weight[p.weight.length - 1].value, u)}</span><span className="stat-s small">from Pocket Health</span></div>}
      </div>
      <Card title="Workouts per week" sub={p.health ? 'Dark: your workouts here · light: runs, rides and walks from Pocket Health' : undefined}>
        <Bars label="Workouts per week" goal={p.goal} rows={p.weeks.map(w => ({ key: w.start, label: new Date(w.start + 'T12:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short' }), a: w.workouts, b: w.health,
          title: `Week of ${fmtDate(w.start, false)}: ${w.workouts} workouts${w.health ? ` + ${w.health} from Pocket Health` : ''}, ${w.sets} sets, ${tonnage(w.volume, u)}` }))} />
      </Card>
      <div className="two-grid">
        <Card title="Sets per muscle" sub="The last 30 days; other muscles an exercise works count half.">
          {p.muscles.length ? <ul className="plain muscle-bars">{p.muscles.map(m => (
            <li key={m.muscle}><span className="mb-name">{muscleLabel(m.muscle)}</span><span className="mb-track"><span style={{ width: `${m.sets / maxM * 100}%` }} /></span><span className="mb-n tnum">{m.sets}</span></li>
          ))}</ul> : <p className="muted small">No sets in the last 30 days.</p>}
        </Card>
        <Card title="Latest records" flush>
          {p.records.length ? <ul className="list">{p.records.map((r, i) => (
            <li key={i}><Link to={`/exercises/${r.exercise_id}`} className="list-row">
              <span className="list-ico gold-bg"><Icon name="trophy" size={15} /></span>
              <span className="row-main grow"><span className="row-title">{r.exercise}</span><span className="muted small">{r.text.replace(`${r.exercise}: `, '')}</span></span>
              <span className="muted small">{r.date ? fmtDate(r.date, false) : ''}</span>
            </Link></li>))}</ul> : <p className="muted pad">Beat your best at an exercise and it shows here.</p>}
        </Card>
      </div>
      {p.weight && p.weight.length > 1 && <Card title="Your weight" sub="From Pocket Health, the last six months.">
        <LineChart label="Weight" points={p.weight.map(w => ({ date: w.date, v: w.value }))} fmt={v => weight(v, u)} />
      </Card>}
      {settings?.ai_review && p.totals.workouts > 0 && <AIReview period="month" />}
    </div>
  )
}
