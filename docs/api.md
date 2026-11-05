# API

Everything is under `/api/v1`, JSON in and out, with a session cookie, a personal token (`Authorization: Bearer pfn_…`, Settings → API & shortcuts) or a Pocket Core link token. Data is metric: weights in **kg**, distances in **metres**, times in **seconds**. `started_at`/`ended_at` are RFC 3339 (UTC); `date` is the local `YYYY-MM-DD`. A workout with an empty `ended_at` is still in progress.

Set kinds follow the exercise's `kind`: `weight` (kg × reps), `reps` (bodyweight reps, extra kg optional), `time` (`duration_s`), `cardio` (`distance_m` + `duration_s`). Set `type` is `normal`, `warmup`, `drop` or `failure`; warm-ups never count toward totals or records.

## Reading (`fitness:read` for linked apps)

| | |
|---|---|
| `GET /summary` | the short version (Pocket AI, Pocket Day, Homepage): `week_done`, `week_goal`, `week_start`, `streak_weeks`, `today` (`{routine, routine_id, time, exercises, minutes, done}` or null), `in_progress`, `last_workout`, `records` (latest 3) and a one-line `text` |
| `GET /day/YYYY-MM-DD` | one date: `workouts` (each with `exercises: [{exercise, sets: "80 kg × 8, 80 kg × 8", records}]`, `duration_min`, `sets`, `volume_kg`, `records`, `url`), `other` (Pocket Health's runs and rides), `planned`, and a `text` |
| `GET /week[?date=]` | the week a date is in: each day's workouts (yours and Pocket Health's, `health: true`), what was planned, `done` against `goal` |
| `GET /overview` | the Today page: the workout in progress, today's plan (with the weather for an outdoor routine), the week, recent workouts and records |
| `GET /workouts[?from=&to=&routine=&exercise=&limit=&offset=&sets=1]` | finished workouts, newest first, with totals (`exercises`, `set_count`, `volume` kg, `reps`, `distance_m`, `records`); `sets=1` adds the sets |
| `GET /workouts/{id}` | `{workout (with sets), exercises: {id: exercise}, previous: {exercise id: [last time's sets]}, routine}` |
| `GET /workouts/active` | the workout in progress (`{"workout": null}` when none) |
| `GET /records[?exercise=ID]` | `records`: the best per exercise and kind (`weight`, `e1rm`, `volume`, `reps`, `duration`, `distance`); `recent`: the latest sets that set one |
| `GET /progress[?weeks=12]` | per week workouts, Pocket Health workouts, kg, sets, minutes; sets per muscle (30 days); weight from Pocket Health; totals |
| `GET /routines`, `GET /routines/{id}` | routines with their exercises and aims (`sets`, `reps_min`–`reps_max`, `weight`, `duration_s`, `distance_m`, `rest_s`), `weekdays` they are planned on |
| `GET /plan` | the week: `[{weekday (1 = Monday), routine_id, routine, time}]`; days left out are rest days |
| `GET /exercises[?archived=1]`, `GET /exercises/{id}` | the exercise list (built-in + your own); one with its records, history and chart points |
| `GET /exercises/{id}/history[?limit=]` | its sessions, newest first |

## Logging (`fitness:write` for linked apps)

`POST /workouts/log` records a finished workout in one go — exercises by `exercise_id` or by `name` (matched to yours or the built-in list, `"bench"` → Bench Press (Barbell); unknown names are added as your own):

```json
{
  "name": "Push day",
  "date": "2026-11-05",
  "started_at": "2026-11-05T18:00",
  "duration_min": 55,
  "notes": "",
  "feel": 4,
  "source_ref": "optional: the same ref again updates instead of adding",
  "exercises": [
    {"name": "Bench Press (Barbell)", "sets": [{"weight": 80, "reps": 8}, {"weight": 80, "reps": 8}, {"weight": 80, "reps": 7}]},
    {"name": "Pull Up", "sets": [{"reps": 10}, {"reps": 8}]},
    {"name": "Running", "sets": [{"distance_m": 5000, "duration_s": 1560}]}
  ]
}
```

It answers like finishing a workout: `{workout, exercises, records: [{exercise, kinds, weight, reps, text}], links: {health, habit}, week: {done, goal, met}}`. A linked app may `PUT`/`DELETE /workouts/{id}` only for the workouts it logged itself.

## The page's own calls

| | |
|---|---|
| `POST /workouts` | start one: `{"routine_id": 3}` (sets filled with last time's numbers, else the routine's aim) or `{}` for an empty one; when one is running, that one comes back |
| `PUT /workouts/{id}` | save it as it is: `name`, `notes`, `feel`, `started_at`, `ended_at` (finished ones), `routine_id`, and the whole `sets` list `[{exercise_id, position, type, weight, reps, duration_s, distance_m, rpe, done, note, rest_s}]` |
| `POST /workouts/{id}/finish` | end it (sets not ticked are dropped): records, Pocket Health, Pocket Habits, events |
| `POST /workouts/{id}/resend` | tell Pocket Health and Pocket Habits again |
| `POST /workouts/{id}/routine` | keep a workout as a routine: `{"name": "…"}` |
| `DELETE /workouts/{id}` | |
| `POST /routines`, `PUT /routines/{id}`, `DELETE /routines/{id}`, `POST /routines/order` | `{"name", "notes", "outdoors", "items": [{exercise_id, sets, reps_min, reps_max, weight, duration_s, distance_m, rest_s, notes}]}` |
| `PUT /plan` | `[{"weekday": 1, "routine_id": 3, "time": "18:00"}]` |
| `POST /exercises`, `PUT /exercises/{id}`, `DELETE /exercises/{id}` | `name`, `muscle`, `secondary`, `equipment`, `kind`, `notes`, `archived`; deleting one you did hides it (`{"archived": true}`) |
| `GET /apps/habits`, `/apps/calendars`, `/apps/playlists`, `POST /apps/calendar/sync` | the lists Settings picks from |
| `POST /ai/review` | `{"period": "week"\|"month", "fresh": false}` → `{ok, text}` |
| `POST /ai/routine` | `{"goal", "days", "minutes", "equipment": ["dumbbell"], "level": "new"\|"some"\|"experienced"}` → drafts `{ok, routines: [{name, notes, weekdays, items}]}`, nothing saved |
| `POST /ai/parse` | `{"text": "bench 3x8 at 80, then pull-ups"}` → a draft for `POST /workouts/log`, nothing saved |
| `POST /import[?unit=lb]` | the body is a Strong CSV, a Hevy CSV or a Pocket Fitness `.json` export; importing twice adds nothing |
| `GET /export.json`, `GET /export.csv` | everything / every set (Hevy's columns) |
| `GET /backup` | the whole database (admins) |

## Events (Pocket Core bus)

| Topic | When | Payload (plus `user_email`) |
|---|---|---|
| `fitness.workout_done` | a workout is finished or logged | `workout_id, name, date, started_at, duration_min, exercises, sets, volume_kg, records, url` |
| `fitness.record` | a set beat everything before it | `exercise, kinds, weight_kg, reps, duration_s, distance_m, e1rm_kg, workout_id, date, text` |
| `fitness.goal_met` | the week's goal is reached | `week, done, goal, text` |
| `fitness.plan_today` | the morning of a planned session | `date, routine, routine_id, time, minutes, exercises, text, url` |
