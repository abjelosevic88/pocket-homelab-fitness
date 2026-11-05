package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

const workoutCols = `id, user_id, name, routine_id, started_at, ended_at, date, duration_s, notes, feel, bodyweight, source, source_ref, health_ref, health_error, habit_done, created_at, updated_at`

func scanWorkout(sc interface{ Scan(...any) error }) (*Workout, error) {
	var w Workout
	var rid sql.NullInt64
	var bw sql.NullFloat64
	if err := sc.Scan(&w.ID, &w.UserID, &w.Name, &rid, &w.StartedAt, &w.EndedAt, &w.Date, &w.DurationS, &w.Notes, &w.Feel, &bw, &w.Source, &w.SourceRef,
		&w.HealthRef, &w.HealthError, &w.HabitDone, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	if rid.Valid {
		w.RoutineID = &rid.Int64
	}
	w.Bodyweight = nFloat(bw)
	return &w, nil
}

const setCols = `id, workout_id, exercise_id, position, set_no, type, weight, reps, duration_s, distance_m, rpe, done, note, rest_s`

func scanSet(sc interface{ Scan(...any) error }) (Set, int64, error) {
	var st Set
	var wid int64
	var w, dist, rpe sql.NullFloat64
	var reps, dur, rest sql.NullInt64
	var done int
	if err := sc.Scan(&st.ID, &wid, &st.ExerciseID, &st.Position, &st.SetNo, &st.Type, &w, &reps, &dur, &dist, &rpe, &done, &st.Note, &rest); err != nil {
		return st, 0, err
	}
	st.Weight, st.Reps, st.DurationS, st.DistanceM, st.RPE, st.RestS = nFloat(w), nInt(reps), nInt(dur), nFloat(dist), nFloat(rpe), nInt(rest)
	st.Done = done == 1
	return st, wid, nil
}

// WorkoutFilter narrows a list of workouts.
type WorkoutFilter struct {
	From, To   string // dates, inclusive
	RoutineID  int64
	ExerciseID int64
	Source     string
	Finished   bool // leave the one in progress out
	Limit      int
	Offset     int
}

// ListWorkouts lists a person's workouts, newest first, with their totals.
func (s *Store) ListWorkouts(ctx context.Context, uid int64, f WorkoutFilter) ([]Workout, error) {
	q := `SELECT ` + workoutCols + ` FROM workouts WHERE user_id = ?`
	args := []any{uid}
	if f.From != "" {
		q += ` AND date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		q += ` AND date <= ?`
		args = append(args, f.To)
	}
	if f.RoutineID > 0 {
		q += ` AND routine_id = ?`
		args = append(args, f.RoutineID)
	}
	if f.ExerciseID > 0 {
		q += ` AND id IN (SELECT workout_id FROM workout_sets WHERE user_id = ? AND exercise_id = ?)`
		args = append(args, uid, f.ExerciseID)
	}
	if f.Source != "" {
		q += ` AND source = ?`
		args = append(args, f.Source)
	}
	if f.Finished {
		q += ` AND ended_at <> ''`
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	q += ` ORDER BY date DESC, started_at DESC LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []Workout{}
	for rows.Next() {
		w, err := scanWorkout(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *w)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]any, 0, len(out)+1)
	ids = append(ids, uid)
	for _, w := range out {
		ids = append(ids, w.ID)
	}
	sets, err := s.setsOf(ctx, `user_id = ? AND workout_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(out)), ",")+`)`, ids...)
	if err != nil {
		return nil, err
	}
	prs, _ := s.PRs(ctx, uid)
	for i := range out {
		Totals(&out[i], sets[out[i].ID], prs)
	}
	return out, nil
}

// setsOf loads sets by workout.
func (s *Store) setsOf(ctx context.Context, where string, args ...any) (map[int64][]Set, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+setCols+` FROM workout_sets WHERE `+where+` ORDER BY workout_id, position, set_no, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]Set{}
	for rows.Next() {
		st, wid, err := scanSet(rows)
		if err != nil {
			return nil, err
		}
		out[wid] = append(out[wid], st)
	}
	return out, rows.Err()
}

// Totals fills a workout's totals from its sets (and marks their records).
func Totals(w *Workout, sets []Set, prs map[int64][]string) {
	ex := map[int64]bool{}
	w.SetCount, w.Volume, w.Reps, w.DistanceM, w.Records = 0, 0, 0, 0, 0
	for i := range sets {
		st := &sets[i]
		st.PR = prs[st.ID]
		if len(st.PR) > 0 {
			w.Records++
		}
		ex[st.ExerciseID] = true
		if !st.Counts() {
			continue
		}
		w.SetCount++
		w.Volume += st.Volume()
		if st.Reps != nil {
			w.Reps += *st.Reps
		}
		if st.DistanceM != nil {
			w.DistanceM += *st.DistanceM
		}
	}
	w.Exercises = len(ex)
	w.Volume = float64(int64(w.Volume*10+0.5)) / 10
	w.Sets = sets
}

// GetWorkout returns one of a person's workouts with its sets.
func (s *Store) GetWorkout(ctx context.Context, uid, id int64) (*Workout, error) {
	w, err := scanWorkout(s.DB.QueryRowContext(ctx, `SELECT `+workoutCols+` FROM workouts WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	sets, err := s.setsOf(ctx, `user_id = ? AND workout_id = ?`, uid, id)
	if err != nil {
		return nil, err
	}
	prs, _ := s.PRs(ctx, uid)
	Totals(w, sets[id], prs)
	if w.Sets == nil {
		w.Sets = []Set{}
	}
	return w, nil
}

// ActiveWorkout returns the workout a person has in progress, if any.
func (s *Store) ActiveWorkout(ctx context.Context, uid int64) (*Workout, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM workouts WHERE user_id = ? AND ended_at = '' ORDER BY started_at DESC LIMIT 1`, uid).Scan(&id)
	if err != nil {
		return nil, notFound(err)
	}
	return s.GetWorkout(ctx, uid, id)
}

// SaveWorkout creates (ID 0) or updates a workout and replaces its sets.
// The sets' exercises must be the person's own (checked by the caller).
func (s *Store) SaveWorkout(ctx context.Context, w *Workout) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var rid any
	if w.RoutineID != nil && *w.RoutineID > 0 {
		rid = *w.RoutineID
	}
	if w.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO workouts (user_id, name, routine_id, started_at, ended_at, date, duration_s, notes, feel, bodyweight, source, source_ref)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, w.UserID, w.Name, rid, w.StartedAt, w.EndedAt, w.Date, w.DurationS, w.Notes, w.Feel, pFloat(w.Bodyweight), w.Source, w.SourceRef)
		if err != nil {
			return conflict(err)
		}
		w.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE workouts SET name = ?, routine_id = ?, started_at = ?, ended_at = ?, date = ?, duration_s = ?, notes = ?, feel = ?, bodyweight = ?, updated_at = ?
			WHERE user_id = ? AND id = ?`, w.Name, rid, w.StartedAt, w.EndedAt, w.Date, w.DurationS, w.Notes, w.Feel, pFloat(w.Bodyweight), Now(), w.UserID, w.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workout_sets WHERE user_id = ? AND workout_id = ?`, w.UserID, w.ID); err != nil {
			return err
		}
	}
	for _, st := range w.Sets {
		if st.Type == "" {
			st.Type = "normal"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workout_sets (user_id, workout_id, exercise_id, position, set_no, type, weight, reps, duration_s, distance_m, rpe, done, note, rest_s)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, w.UserID, w.ID, st.ExerciseID, st.Position, st.SetNo, st.Type, pFloat(st.Weight), pInt(st.Reps),
			pInt(st.DurationS), pFloat(st.DistanceM), pFloat(st.RPE), boolInt(st.Done), st.Note, pInt(st.RestS)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetWorkoutLinks saves what the other apps made of a workout.
func (s *Store) SetWorkoutLinks(ctx context.Context, uid, id int64, healthRef, healthErr, habitDone string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE workouts SET health_ref = ?, health_error = ?, habit_done = ? WHERE user_id = ? AND id = ?`, healthRef, healthErr, habitDone, uid, id)
	return err
}

// DeleteWorkout removes a workout and its sets.
func (s *Store) DeleteWorkout(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM workouts WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// WorkoutBySource finds a workout another app or an import made.
func (s *Store) WorkoutBySource(ctx context.Context, uid int64, source, ref string) (*Workout, error) {
	var id int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM workouts WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref).Scan(&id); err != nil {
		return nil, notFound(err)
	}
	return s.GetWorkout(ctx, uid, id)
}

// Previous returns, per exercise, the sets of the last workout before this
// one that had it ("last time: 80 kg × 8").
func (s *Store) Previous(ctx context.Context, uid int64, exerciseIDs []int64, beforeStarted string, skipWorkout int64) (map[int64][]Set, error) {
	out := map[int64][]Set{}
	for _, ex := range exerciseIDs {
		if _, ok := out[ex]; ok {
			continue
		}
		var wid int64
		err := s.DB.QueryRowContext(ctx, `SELECT w.id FROM workouts w JOIN workout_sets ws ON ws.workout_id = w.id
			WHERE w.user_id = ? AND ws.exercise_id = ? AND w.id <> ? AND w.started_at < ? AND ws.done = 1 ORDER BY w.started_at DESC LIMIT 1`, uid, ex, skipWorkout, beforeStarted).Scan(&wid)
		if err != nil {
			out[ex] = []Set{}
			continue
		}
		sets, err := s.setsOf(ctx, `user_id = ? AND workout_id = ? AND exercise_id = ? AND done = 1`, uid, wid, ex)
		if err != nil {
			return nil, err
		}
		out[ex] = sets[wid]
	}
	return out, nil
}

// flatSet is one done set with its workout, for the record scans.
type flatSet struct {
	Set
	workout int64
	date    string
	kind    string
}

// doneSets returns every done, non-warm-up set of a person, oldest first.
func (s *Store) doneSets(ctx context.Context, uid int64, exerciseID int64) ([]flatSet, map[int64]*Exercise, error) {
	ex, err := s.ExercisesByID(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	q := `SELECT ws.id, ws.workout_id, ws.exercise_id, ws.position, ws.set_no, ws.type, ws.weight, ws.reps, ws.duration_s, ws.distance_m, ws.rpe, ws.done, ws.note, ws.rest_s, w.date
		FROM workout_sets ws JOIN workouts w ON w.id = ws.workout_id WHERE ws.user_id = ? AND ws.done = 1 AND ws.type <> 'warmup'`
	args := []any{uid}
	if exerciseID > 0 {
		q += ` AND ws.exercise_id = ?`
		args = append(args, exerciseID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY w.started_at, ws.position, ws.set_no, ws.id`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []flatSet
	for rows.Next() {
		var f flatSet
		var w, dist, rpe sql.NullFloat64
		var reps, dur, rest sql.NullInt64
		var done int
		if err := rows.Scan(&f.ID, &f.workout, &f.ExerciseID, &f.Position, &f.SetNo, &f.Type, &w, &reps, &dur, &dist, &rpe, &done, &f.Note, &rest, &f.date); err != nil {
			return nil, nil, err
		}
		f.Weight, f.Reps, f.DurationS, f.DistanceM, f.RPE, f.RestS = nFloat(w), nInt(reps), nInt(dur), nFloat(dist), nFloat(rpe), nInt(rest)
		f.Done = true
		if e := ex[f.ExerciseID]; e != nil {
			f.kind = e.Kind
		}
		out = append(out, f)
	}
	return out, ex, rows.Err()
}

// RecordKinds are the ways a set can be a record, per kind of exercise.
var RecordKinds = map[string][]string{
	"weight": {"weight", "e1rm", "volume"},
	"reps":   {"reps"},
	"time":   {"duration"},
	"cardio": {"distance"},
}

// SetValue is a set's value for one kind of record (0 = does not apply).
func SetValue(st Set, kind string) float64 {
	switch kind {
	case "weight":
		if st.Weight != nil && st.Reps != nil && *st.Reps > 0 {
			return *st.Weight
		}
	case "e1rm":
		return st.E1RM()
	case "volume":
		return st.Volume()
	case "reps":
		if st.Reps != nil {
			return float64(*st.Reps)
		}
	case "duration":
		if st.DurationS != nil {
			return float64(*st.DurationS)
		}
	case "distance":
		if st.DistanceM != nil {
			return *st.DistanceM
		}
	}
	return 0
}

// PRs works out, over a person's whole history, which sets beat everything
// before them (set id → record kinds). A workout's best set per kind counts
// once; the first time an exercise is done sets the mark, it is no record.
func (s *Store) PRs(ctx context.Context, uid int64) (map[int64][]string, error) {
	sets, _, err := s.doneSets(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	out := map[int64][]string{}
	type key struct {
		ex   int64
		kind string
	}
	best := map[key]float64{}
	seen := map[int64]bool{} // exercises done in an earlier workout
	for i := 0; i < len(sets); {
		j := i
		for j < len(sets) && sets[j].workout == sets[i].workout {
			j++
		}
		// within one workout: its best set per exercise and kind
		top := map[key]flatSet{}
		for _, f := range sets[i:j] {
			for _, k := range RecordKinds[f.kind] {
				v := SetValue(f.Set, k)
				kk := key{f.ExerciseID, k}
				if v > 0 && v > SetValue(top[kk].Set, k) {
					top[kk] = f
				}
			}
		}
		for kk, f := range top {
			v := SetValue(f.Set, kk.kind)
			if seen[kk.ex] && v > best[kk] {
				out[f.ID] = append(out[f.ID], kk.kind)
			}
			if v > best[kk] {
				best[kk] = v
			}
		}
		for _, f := range sets[i:j] {
			seen[f.ExerciseID] = true
		}
		i = j
	}
	for id := range out {
		sort.Slice(out[id], func(a, b int) bool { return recordOrder(out[id][a]) < recordOrder(out[id][b]) })
	}
	return out, nil
}

func recordOrder(k string) int {
	return map[string]int{"weight": 0, "e1rm": 1, "reps": 2, "volume": 3, "duration": 4, "distance": 5}[k]
}

// Records returns a person's best at every exercise (one exercise: 0 = all).
func (s *Store) Records(ctx context.Context, uid, exerciseID int64) ([]Record, error) {
	sets, ex, err := s.doneSets(ctx, uid, exerciseID)
	if err != nil {
		return nil, err
	}
	type key struct {
		ex   int64
		kind string
	}
	best := map[key]flatSet{}
	for _, f := range sets {
		for _, k := range RecordKinds[f.kind] {
			kk := key{f.ExerciseID, k}
			if v := SetValue(f.Set, k); v > 0 && v > SetValue(best[kk].Set, k) {
				best[kk] = f
			}
		}
	}
	out := []Record{}
	for kk, f := range best {
		r := Record{ExerciseID: kk.ex, Kind: kk.kind, Value: SetValue(f.Set, kk.kind), WorkoutID: f.workout, Date: f.date}
		if e := ex[kk.ex]; e != nil {
			r.Exercise = e.Name
		}
		if f.Weight != nil {
			r.Weight = *f.Weight
		}
		if f.Reps != nil {
			r.Reps = *f.Reps
		}
		r.Value = float64(int64(r.Value*10+0.5)) / 10
		out = append(out, r)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Exercise != out[b].Exercise {
			return strings.ToLower(out[a].Exercise) < strings.ToLower(out[b].Exercise)
		}
		return recordOrder(out[a].Kind) < recordOrder(out[b].Kind)
	})
	return out, nil
}

// Session is one workout's sets of one exercise, for its history.
type Session struct {
	WorkoutID int64   `json:"workout_id"`
	Workout   string  `json:"workout"`
	Date      string  `json:"date"`
	Sets      []Set   `json:"sets"`
	Best      float64 `json:"best"`   // heaviest weight / most reps / longest / furthest
	E1RM      float64 `json:"e1rm"`   // best estimated one-rep max
	Volume    float64 `json:"volume"` // kg
}

// ExerciseHistory returns the sessions of one exercise, newest first.
func (s *Store) ExerciseHistory(ctx context.Context, uid, exerciseID int64, limit int) ([]Session, error) {
	e, err := s.GetExercise(ctx, uid, exerciseID)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT w.id, w.name, w.date, w.started_at FROM workouts w JOIN workout_sets ws ON ws.workout_id = w.id
		WHERE w.user_id = ? AND ws.exercise_id = ? AND ws.done = 1 ORDER BY w.started_at DESC LIMIT ?`, uid, exerciseID, limit)
	if err != nil {
		return nil, err
	}
	var out []Session
	for rows.Next() {
		var ss Session
		var started string
		if err := rows.Scan(&ss.WorkoutID, &ss.Workout, &ss.Date, &started); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, ss)
	}
	rows.Close()
	prs, _ := s.PRs(ctx, uid)
	mainKind := RecordKinds[e.Kind][0]
	for i := range out {
		sets, err := s.setsOf(ctx, `user_id = ? AND workout_id = ? AND exercise_id = ? AND done = 1`, uid, out[i].WorkoutID, exerciseID)
		if err != nil {
			return nil, err
		}
		out[i].Sets = sets[out[i].WorkoutID]
		for j := range out[i].Sets {
			st := &out[i].Sets[j]
			st.PR = prs[st.ID]
			if !st.Counts() {
				continue
			}
			if v := SetValue(*st, mainKind); v > out[i].Best {
				out[i].Best = v
			}
			if v := st.E1RM(); v > out[i].E1RM {
				out[i].E1RM = float64(int64(v*10+0.5)) / 10
			}
			out[i].Volume += st.Volume()
		}
	}
	if out == nil {
		out = []Session{}
	}
	return out, nil
}

// DayCount is how many workouts a person did on a date.
type DayCount struct {
	Date    string   `json:"date"`
	N       int      `json:"n"`
	Names   []string `json:"names"`
	Minutes int      `json:"minutes"`
}

// WorkoutDays returns the days with finished workouts in a range.
func (s *Store) WorkoutDays(ctx context.Context, uid int64, from, to string) (map[string]*DayCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, name, duration_s FROM workouts WHERE user_id = ? AND ended_at <> '' AND date >= ? AND date <= ? ORDER BY started_at`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*DayCount{}
	for rows.Next() {
		var d, name string
		var dur int
		if err := rows.Scan(&d, &name, &dur); err != nil {
			return nil, err
		}
		dc := out[d]
		if dc == nil {
			dc = &DayCount{Date: d}
			out[d] = dc
		}
		dc.N++
		dc.Names = append(dc.Names, name)
		dc.Minutes += dur / 60
	}
	return out, rows.Err()
}

// MuscleSets counts the done, non-warm-up sets per muscle in a range
// (the exercise's main muscle counts one, each other muscle half).
func (s *Store) MuscleSets(ctx context.Context, uid int64, from, to string) (map[string]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.muscle, e.secondary, COUNT(1) FROM workout_sets ws JOIN workouts w ON w.id = ws.workout_id JOIN exercises e ON e.id = ws.exercise_id
		WHERE ws.user_id = ? AND ws.done = 1 AND ws.type <> 'warmup' AND w.date >= ? AND w.date <= ? GROUP BY e.muscle, e.secondary`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var m, sec string
		var n int
		if err := rows.Scan(&m, &sec, &n); err != nil {
			return nil, err
		}
		out[m] += float64(n)
		for _, x := range strings.Split(sec, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out[x] += float64(n) / 2
			}
		}
	}
	return out, rows.Err()
}

// WeekVolume returns the kg lifted and the sets done per week (keyed by the
// week's first day) in a range.
func (s *Store) WeekVolume(ctx context.Context, uid int64, from, to string, weekStart int) (map[string][2]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT w.date, COALESCE(SUM(CASE WHEN ws.weight IS NOT NULL AND ws.reps IS NOT NULL THEN ws.weight * ws.reps ELSE 0 END), 0), COUNT(ws.id)
		FROM workouts w LEFT JOIN workout_sets ws ON ws.workout_id = w.id AND ws.done = 1 AND ws.type <> 'warmup'
		WHERE w.user_id = ? AND w.ended_at <> '' AND w.date >= ? AND w.date <= ? GROUP BY w.date`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][2]float64{}
	for rows.Next() {
		var d string
		var v float64
		var n int
		if err := rows.Scan(&d, &v, &n); err != nil {
			return nil, err
		}
		wk := WeekOf(d, weekStart)
		cur := out[wk]
		out[wk] = [2]float64{cur[0] + v, cur[1] + float64(n)}
	}
	return out, rows.Err()
}

// WeekOf is the first day of the week a date is in (weekStart 1 = Monday,
// 7 = Sunday, 6 = Saturday).
func WeekOf(date string, weekStart int) string {
	t, err := ParseDate(date)
	if err != nil {
		return date
	}
	wd := int(t.Weekday()) // 0 = Sunday
	if weekStart < 1 || weekStart > 7 {
		weekStart = 1
	}
	start := weekStart % 7
	back := (wd - start + 7) % 7
	return t.AddDate(0, 0, -back).Format("2006-01-02")
}

// AllWorkouts returns every workout of a person with its sets, oldest first (export).
func (s *Store) AllWorkouts(ctx context.Context, uid int64) ([]Workout, error) {
	list, err := s.ListWorkouts(ctx, uid, WorkoutFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	for off := 1000; len(list) == off; off += 1000 {
		more, err := s.ListWorkouts(ctx, uid, WorkoutFilter{Limit: 1000, Offset: off})
		if err != nil {
			return nil, err
		}
		list = append(list, more...)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].StartedAt < list[b].StartedAt })
	return list, nil
}
