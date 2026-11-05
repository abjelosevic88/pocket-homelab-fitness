CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);

-- ---------- Pocket Fitness: everything below is per person ----------

-- The exercises a person picks from: the built-in list (builtin = its key,
-- copied into each account the first time it is used, so it can be renamed
-- or hidden) and their own. Kind decides what a set holds.
CREATE TABLE exercises (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,                         -- "Bench press (barbell)"
  builtin      TEXT NOT NULL DEFAULT '',              -- catalog key, '' = your own
  muscle       TEXT NOT NULL DEFAULT 'other',         -- chest | back | shoulders | biceps | triceps | forearms | core | quads | hamstrings | glutes | calves | full_body | cardio | other
  secondary    TEXT NOT NULL DEFAULT '',              -- other muscles, comma separated
  equipment    TEXT NOT NULL DEFAULT 'other',         -- barbell | dumbbell | machine | cable | bodyweight | kettlebell | band | cardio | other
  kind         TEXT NOT NULL DEFAULT 'weight',        -- weight (kg × reps) | reps (bodyweight reps, extra kg optional) | time (seconds) | cardio (distance + time)
  notes        TEXT NOT NULL DEFAULT '',
  archived     INTEGER NOT NULL DEFAULT 0,            -- hidden from the pickers, history kept
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX exercises_name ON exercises(user_id, name COLLATE NOCASE);
CREATE INDEX exercises_builtin ON exercises(user_id, builtin);

-- A routine: the exercises of a session with what to aim for ("Push day").
CREATE TABLE routines (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  notes        TEXT NOT NULL DEFAULT '',
  outdoors     INTEGER NOT NULL DEFAULT 0,            -- a run in the park: the weather matters
  sort         INTEGER NOT NULL DEFAULT 0,
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | ai | import
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX routines_user ON routines(user_id, sort, name);

CREATE TABLE routine_items (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  routine_id   INTEGER NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
  exercise_id  INTEGER NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL DEFAULT 0,
  sets         INTEGER NOT NULL DEFAULT 3,
  reps_min     INTEGER,                               -- 8 of "8–12"
  reps_max     INTEGER,
  weight       REAL,                                  -- kg to aim for ('' = what you did last time)
  duration_s   INTEGER,
  distance_m   REAL,
  rest_s       INTEGER,                               -- '' = your default rest
  notes        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX routine_items_routine ON routine_items(user_id, routine_id, position);

-- The week: which routine on which day, and when.
CREATE TABLE plan (
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  weekday      INTEGER NOT NULL,                      -- 1 = Monday … 7 = Sunday
  routine_id   INTEGER NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
  time         TEXT NOT NULL DEFAULT '',              -- HH:MM, '' = some time that day
  PRIMARY KEY (user_id, weekday)
);

-- Planned sessions put on Pocket Calendar, so they can be moved or removed
-- when the plan changes.
CREATE TABLE plan_events (
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  routine_id   INTEGER NOT NULL DEFAULT 0,
  time         TEXT NOT NULL DEFAULT '',
  event_id     INTEGER NOT NULL,
  PRIMARY KEY (user_id, date)
);

-- A workout: one session, in progress until it has an end.
CREATE TABLE workouts (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  routine_id   INTEGER REFERENCES routines(id) ON DELETE SET NULL,
  started_at   TEXT NOT NULL,                         -- RFC 3339, UTC
  ended_at     TEXT NOT NULL DEFAULT '',              -- '' = in progress
  date         TEXT NOT NULL,                         -- YYYY-MM-DD, local
  duration_s   INTEGER NOT NULL DEFAULT 0,
  notes        TEXT NOT NULL DEFAULT '',
  feel         INTEGER NOT NULL DEFAULT 0,            -- 1 (hard going) … 5 (great), 0 = not said
  bodyweight   REAL,                                  -- kg that day (Pocket Health or your own)
  source       TEXT NOT NULL DEFAULT '',              -- '' by hand | import | strong | hevy | ai | <app slug>
  source_ref   TEXT NOT NULL DEFAULT '',
  health_ref   TEXT NOT NULL DEFAULT '',              -- the workout's id in Pocket Health once sent
  health_error TEXT NOT NULL DEFAULT '',
  habit_done   TEXT NOT NULL DEFAULT '',              -- the date ticked in Pocket Habits
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX workouts_user_date ON workouts(user_id, date DESC, started_at DESC);
CREATE UNIQUE INDEX workouts_source ON workouts(user_id, source, source_ref) WHERE source_ref <> '';

-- The sets of a workout, exercise by exercise (position) and set by set.
CREATE TABLE workout_sets (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workout_id   INTEGER NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
  exercise_id  INTEGER NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL DEFAULT 0,            -- the exercise's place in the workout
  set_no       INTEGER NOT NULL DEFAULT 0,
  type         TEXT NOT NULL DEFAULT 'normal',        -- normal | warmup | drop | failure
  weight       REAL,                                  -- kg (extra kg for bodyweight exercises)
  reps         INTEGER,
  duration_s   INTEGER,
  distance_m   REAL,
  rpe          REAL,                                  -- how hard, 6–10
  done         INTEGER NOT NULL DEFAULT 1,
  note         TEXT NOT NULL DEFAULT '',
  rest_s       INTEGER                                -- rest after this exercise's sets (live workouts)
);
CREATE INDEX workout_sets_workout ON workout_sets(user_id, workout_id, position, set_no);
CREATE INDEX workout_sets_exercise ON workout_sets(user_id, exercise_id);
