// Every finished workout, newest first, by month; filter by routine.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useData, useOnChange } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Workout } from '../lib/types'
import { distance, mins, tonnage, unitsOf } from '../lib/units'
import { openLog } from '../components/Layout'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { Card, Empty, Loading, Spinner } from '../components/ui'

const PAGE = 50

export default function History() {
  const { settings } = useApp()
  const { overview } = useData()
  const u = unitsOf(settings)
  const [routine, setRoutine] = useState('')
  const [list, setList] = useState<Workout[] | null>(null)
  const [more, setMore] = useState(false)
  const [busy, setBusy] = useState(false)
  const load = useCallback((offset = 0) => {
    setBusy(true)
    api.get<Workout[]>(`${V1}/workouts?limit=${PAGE}&offset=${offset}${routine ? `&routine=${routine}` : ''}`)
      .then(l => { setList(prev => offset ? [...(prev || []), ...l] : l); setMore(l.length === PAGE) }).finally(() => setBusy(false))
  }, [routine])
  useEffect(() => { load(0) }, [load])
  useOnChange(() => load(0))
  const months: { key: string; items: Workout[] }[] = []
  for (const w of list || []) {
    const k = w.date.slice(0, 7)
    if (!months.length || months[months.length - 1].key !== k) months.push({ key: k, items: [] })
    months[months.length - 1].items.push(w)
  }
  return (
    <div className="stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">History</h1><p className="muted">Every workout you finished.</p></div>
        <div className="page-pick"><Select value={routine} onChange={setRoutine} placeholder="All routines" clearLabel="All routines" ariaLabel="Routine"
          options={(overview?.routines || []).map(r => ({ value: String(r.id), label: r.name }))} /></div>
        <button type="button" className="btn" onClick={openLog}><Icon name="pen" size={15} />Log a workout</button>
      </div>
      {!list ? <Loading /> : !list.length ? <Card><Empty title="No workouts yet" icon="journal">Finished workouts land here. You can also log one you already did, or import from Strong or Hevy in <Link to="/settings/data">Settings</Link>.</Empty></Card> : months.map(m => {
        const vol = m.items.reduce((t, w) => t + w.volume, 0)
        return (
          <Card key={m.key} title={new Date(m.key + '-01T12:00').toLocaleDateString(undefined, { month: 'long', year: 'numeric' })} sub={`${m.items.length} ${m.items.length === 1 ? 'workout' : 'workouts'}${vol > 0 ? ` · ${tonnage(vol, u)}` : ''}`} flush>
            <ul className="list">{m.items.map(w => (
              <li key={w.id}><Link to={`/workouts/${w.id}`} className="list-row">
                <span className="list-ico date-ico"><b>{Number(w.date.slice(8))}</b><span>{new Date(w.date + 'T12:00').toLocaleDateString(undefined, { weekday: 'short' })}</span></span>
                <span className="row-main grow"><span className="row-title">{w.name}{w.records > 0 && <span className="pr-mini" title={`${w.records} records`}><Icon name="trophy" size={12} />{w.records}</span>}</span>
                  <span className="muted small">{mins(Math.round(w.duration_s / 60))} · {w.exercises} exercises · {w.set_count} sets{w.volume > 0 ? ` · ${tonnage(w.volume, u)}` : ''}{w.distance_m > 0 ? ` · ${distance(w.distance_m, u)}` : ''}</span></span>
                <span className="muted small hide-xs">{fmtDate(w.date, false)}</span>
              </Link></li>))}</ul>
          </Card>
        )
      })}
      {more && <button type="button" className="btn self-center" disabled={busy} onClick={() => load(list!.length)}>{busy ? <Spinner /> : null}Show older workouts</button>}
    </div>
  )
}
