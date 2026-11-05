// Today's overview (the workout in progress, the plan, the week, routines)
// and the exercise list, loaded once for the sidebar and the pages; any
// change in the app ("pfn:changed") reloads them.
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { api, V1 } from './api'
import type { Exercise, Overview } from './types'

interface Data {
  overview: Overview | null; exercises: Exercise[]; byId: Record<number, Exercise>; error: string
  reload: () => void; reloadExercises: () => void
}
const Ctx = createContext<Data>({ overview: null, exercises: [], byId: {}, error: '', reload: () => {}, reloadExercises: () => {} })
export const useData = () => useContext(Ctx)

/** Something was added or changed: the overview and pages reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pfn:changed'))
export function useOnChange(cb: () => void) {
  const ref = React.useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pfn:changed', h)
    return () => window.removeEventListener('pfn:changed', h)
  }, [])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [overview, setOverview] = useState<Overview | null>(null)
  const [exercises, setExercises] = useState<Exercise[]>([])
  const [error, setError] = useState('')
  const reload = useCallback(() => {
    api.get<Overview>(`${V1}/overview`).then(o => { setOverview(o); setError('') }).catch(e => setError((e as Error).message))
  }, [])
  const reloadExercises = useCallback(() => {
    api.get<Exercise[]>(`${V1}/exercises?archived=1`).then(setExercises).catch(() => {})
  }, [])
  useEffect(() => {
    reload()
    reloadExercises()
    const vis = () => { if (document.visibilityState === 'visible') reload() }
    document.addEventListener('visibilitychange', vis)
    return () => document.removeEventListener('visibilitychange', vis)
  }, [reload, reloadExercises])
  useOnChange(() => { reload(); reloadExercises() })
  const byId = useMemo(() => Object.fromEntries(exercises.map(e => [e.id, e])), [exercises])
  return <Ctx.Provider value={{ overview, exercises, byId, error, reload, reloadExercises }}>{children}</Ctx.Provider>
}

export const MUSCLES: { id: string; label: string }[] = [
  { id: 'chest', label: 'Chest' }, { id: 'back', label: 'Back' }, { id: 'shoulders', label: 'Shoulders' }, { id: 'biceps', label: 'Biceps' },
  { id: 'triceps', label: 'Triceps' }, { id: 'forearms', label: 'Forearms' }, { id: 'core', label: 'Core' }, { id: 'quads', label: 'Quads' },
  { id: 'hamstrings', label: 'Hamstrings' }, { id: 'glutes', label: 'Glutes' }, { id: 'calves', label: 'Calves' }, { id: 'full_body', label: 'Full body' },
  { id: 'cardio', label: 'Cardio' }, { id: 'other', label: 'Other' },
]
export const muscleLabel = (m: string) => MUSCLES.find(x => x.id === m)?.label || m
export const EQUIPMENT: { id: string; label: string }[] = [
  { id: 'barbell', label: 'Barbell' }, { id: 'dumbbell', label: 'Dumbbells' }, { id: 'machine', label: 'Machine' }, { id: 'cable', label: 'Cable' },
  { id: 'bodyweight', label: 'Bodyweight' }, { id: 'kettlebell', label: 'Kettlebell' }, { id: 'band', label: 'Band' }, { id: 'cardio', label: 'Cardio machine' }, { id: 'other', label: 'Other' },
]
export const KINDS: { id: string; label: string; help: string }[] = [
  { id: 'weight', label: 'Weight and reps', help: 'kg × reps' }, { id: 'reps', label: 'Reps', help: 'bodyweight, extra weight optional' },
  { id: 'time', label: 'Time', help: 'a plank, a hang' }, { id: 'cardio', label: 'Distance and time', help: 'a run, a row' },
]
/** The icon of a kind of workout (yours or Pocket Health's). */
export function kindIcon(kind: string): string {
  switch (kind) {
    case 'running': return 'run'
    case 'walking': case 'hiking': return 'footprints'
    case 'cycling': return 'bike'
    case 'swimming': return 'swim'
    case 'yoga': case 'pilates': case 'stretching': return 'yoga'
    case 'workout': case 'strength': return 'dumbbell'
  }
  return 'activity'
}
