export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'health' | 'habits' | 'calendar' | 'weather' | 'music'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type WeightUnit = 'kg' | 'lb'
export type DistanceUnit = 'km' | 'mi'

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  weight_unit: WeightUnit; distance_unit: DistanceUnit; week_goal: number; rest_s: number; bodyweight_kg: number; plates: boolean; bar_kg: number
  notify_plan: boolean; notify_week: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  health_send: boolean; health_count: boolean; health_weight: boolean; habit_id: number; habit_name: string
  calendar_plan: boolean; calendar_id: number; outdoor_weather: boolean; playlist_id: string; playlist_name: string
  ai_review: boolean; demo_apps: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string
}

export type Muscle = 'chest' | 'back' | 'shoulders' | 'biceps' | 'triceps' | 'forearms' | 'core' | 'quads' | 'hamstrings' | 'glutes' | 'calves' | 'full_body' | 'cardio' | 'other'
export type Equipment = 'barbell' | 'dumbbell' | 'machine' | 'cable' | 'bodyweight' | 'kettlebell' | 'band' | 'cardio' | 'other'
export type ExerciseKind = 'weight' | 'reps' | 'time' | 'cardio'
export type SetType = 'normal' | 'warmup' | 'drop' | 'failure'
export type RecordKind = 'weight' | 'e1rm' | 'volume' | 'reps' | 'duration' | 'distance'

export interface Exercise {
  id: number; name: string; builtin: string; muscle: Muscle; secondary: string; equipment: Equipment; kind: ExerciseKind
  notes: string; archived: boolean; created_at: string; updated_at: string; sets?: number; last_done?: string
}

export interface RoutineItem {
  id?: number; exercise_id: number; position?: number; sets: number; reps_min: number | null; reps_max: number | null; weight: number | null
  duration_s: number | null; distance_m: number | null; rest_s: number | null; notes: string; exercise?: Exercise
}

export interface Routine {
  id: number; name: string; notes: string; outdoors: boolean; sort: number; source: string; items: RoutineItem[]
  created_at: string; updated_at: string; last_done?: string; times?: number; weekdays?: number[]
}

export interface PlanDay { weekday: number; routine_id: number; time: string; routine?: string }

export interface WSet {
  id?: number; exercise_id: number; position: number; set_no?: number; type: SetType
  weight: number | null; reps: number | null; duration_s: number | null; distance_m: number | null; rpe: number | null
  done: boolean; note: string; rest_s: number | null; pr?: RecordKind[]
}

export interface Workout {
  id: number; name: string; routine_id: number | null; started_at: string; ended_at: string; date: string; duration_s: number
  notes: string; feel: number; bodyweight: number | null; source: string; source_ref: string
  health_ref: string; health_error: string; habit_done: string; created_at: string; updated_at: string
  sets?: WSet[]; exercises: number; set_count: number; volume: number; reps: number; distance_m: number; records: number
}

export interface WorkoutFull {
  workout: Workout; exercises: Record<string, Exercise>; previous: Record<string, WSet[]>
  routine?: { id: number; name: string; outdoors: boolean }
}

export interface NewRecord { exercise_id: number; exercise: string; kinds: RecordKind[]; weight?: number; reps?: number; duration_s?: number; distance_m?: number; e1rm?: number; text: string; workout_id?: number; date?: string }
export interface LinkResult { state: 'done' | 'off' | 'error'; detail?: string }
export interface Finished extends WorkoutFull { records: NewRecord[]; links: Record<string, LinkResult>; week: { done: number; goal: number; met: boolean } }

export interface WeekItem { id: number; name: string; kind: string; minutes: number; distance_m?: number; sets?: number; volume?: number; records?: number; health?: boolean }
export interface WeekDay { date: string; items: WeekItem[]; planned: PlanDay | null; upcoming: boolean }
export interface Week { start: string; end: string; days: WeekDay[]; done: number; goal: number; health: boolean }

export interface WeatherDay { date: string; text: string; place: string }
export interface TodayPlan { routine: Routine; time: string; minutes: number; done: boolean; weather?: WeatherDay }

export interface Overview {
  date: string; active: Workout | null; today: TodayPlan | null; week: Week; recent: Workout[]; records: NewRecord[]
  streak: number; routines: Routine[]; weight?: { date: string; value: number }; playlist: { name: string; url: string } | null
}

export interface RecordRow { exercise_id: number; exercise: string; kind: RecordKind; value: number; weight?: number; reps?: number; workout_id: number; date: string }

export interface Session { workout_id: number; workout: string; date: string; sets: WSet[]; best: number; e1rm: number; volume: number }
