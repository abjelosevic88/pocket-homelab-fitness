// "Start a workout": today's planned routine first, your other routines,
// or an empty one; and the way to log one you already did.
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import { fmtClock, fmtDate } from '../lib/format'
import type { WorkoutFull } from '../lib/types'
import { mins } from '../lib/units'
import { Icon } from './icons'
import { errMsg, Modal, Spinner, useToast } from './ui'

export function useStart() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const [busy, setBusy] = useState<number | 'empty' | null>(null)
  const start = async (routineId?: number) => {
    setBusy(routineId || 'empty')
    try {
      const w = await api.post<WorkoutFull>(`${V1}/workouts`, routineId ? { routine_id: routineId } : {})
      announceChange()
      navigate(`/workouts/${w.workout.id}`)
      return true
    } catch (e) { toast(errMsg(e), 'err'); return false } finally { setBusy(null) }
  }
  return { start, busy }
}

export function StartSheet({ onClose, onLog }: { onClose: () => void; onLog: () => void }) {
  const { overview } = useData()
  const { start, busy } = useStart()
  const today = overview?.today && !overview.today.done ? overview.today : null
  const routines = (overview?.routines || []).filter(r => r.id !== today?.routine.id)
  const go = async (id?: number) => { if (await start(id)) onClose() }
  return (
    <Modal title="Start a workout" onClose={onClose} size="sm" className="start-sheet">
      <div className="stack">
        {today && (
          <button type="button" className="start-row today" onClick={() => go(today.routine.id)} disabled={busy != null}>
            <span className="list-ico accent"><Icon name="calendar-check" size={17} /></span>
            <span className="grow"><b>{today.routine.name}</b><span className="muted small block-line">Today's plan{today.time ? ` · ${fmtClock(today.time)}` : ''} · about {mins(today.minutes)}</span></span>
            {busy === today.routine.id ? <Spinner /> : <Icon name="play" size={16} />}
          </button>
        )}
        {routines.length > 0 && <div className="pick-list">
          {routines.map(r => (
            <button key={r.id} type="button" className="start-row" onClick={() => go(r.id)} disabled={busy != null}>
              <span className="list-ico"><Icon name={r.outdoors ? 'sun' : 'dumbbell'} size={16} /></span>
              <span className="grow"><b>{r.name}</b><span className="muted small block-line">{r.items.length} exercises{r.last_done ? ` · last ${fmtDate(r.last_done, false)}` : ''}</span></span>
              {busy === r.id ? <Spinner /> : <Icon name="play" size={15} />}
            </button>
          ))}
        </div>}
        <button type="button" className="start-row" onClick={() => go()} disabled={busy != null}>
          <span className="list-ico"><Icon name="plus" size={16} /></span>
          <span className="grow"><b>Empty workout</b><span className="muted small block-line">Add exercises as you go</span></span>
          {busy === 'empty' ? <Spinner /> : <Icon name="play" size={15} />}
        </button>
        <button type="button" className="link-btn small self-start" onClick={onLog}>Log a workout you already did, or write it in a sentence</button>
      </div>
    </Modal>
  )
}
