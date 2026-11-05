package store

import (
	"context"
	"database/sql"
)

const routineCols = `id, user_id, name, notes, outdoors, sort, source, created_at, updated_at`

func scanRoutine(sc interface{ Scan(...any) error }) (*Routine, error) {
	var r Routine
	var out int
	if err := sc.Scan(&r.ID, &r.UserID, &r.Name, &r.Notes, &out, &r.Sort, &r.Source, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Outdoors = out == 1
	r.Items = []RoutineItem{}
	return &r, nil
}

// ListRoutines lists a person's routines with their exercises, when each was
// last done, how often, and the days of the week it is planned on.
func (s *Store) ListRoutines(ctx context.Context, uid int64) ([]Routine, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+routineCols+` FROM routines WHERE user_id = ? ORDER BY sort, name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	out := []Routine{}
	for rows.Next() {
		r, err := scanRoutine(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items, err := s.routineItems(ctx, uid, 0)
	if err != nil {
		return nil, err
	}
	stats := map[int64][2]string{}
	srows, err := s.DB.QueryContext(ctx, `SELECT routine_id, MAX(date), COUNT(1) FROM workouts WHERE user_id = ? AND routine_id IS NOT NULL AND ended_at <> '' GROUP BY routine_id`, uid)
	if err != nil {
		return nil, err
	}
	times := map[int64]int{}
	for srows.Next() {
		var id int64
		var last string
		var n int
		if err := srows.Scan(&id, &last, &n); err != nil {
			srows.Close()
			return nil, err
		}
		stats[id] = [2]string{last, ""}
		times[id] = n
	}
	srows.Close()
	plan, _ := s.GetPlan(ctx, uid)
	for i := range out {
		out[i].Items = items[out[i].ID]
		if out[i].Items == nil {
			out[i].Items = []RoutineItem{}
		}
		out[i].LastDone = stats[out[i].ID][0]
		out[i].Times = times[out[i].ID]
		for _, p := range plan {
			if p.RoutineID == out[i].ID {
				out[i].Weekdays = append(out[i].Weekdays, p.Weekday)
			}
		}
	}
	return out, nil
}

// routineItems returns the items of one routine (or all, rid 0) by routine.
func (s *Store) routineItems(ctx context.Context, uid, rid int64) (map[int64][]RoutineItem, error) {
	ex, err := s.ExercisesByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	q := `SELECT id, routine_id, exercise_id, position, sets, reps_min, reps_max, weight, duration_s, distance_m, rest_s, notes FROM routine_items WHERE user_id = ?`
	args := []any{uid}
	if rid > 0 {
		q += ` AND routine_id = ?`
		args = append(args, rid)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY routine_id, position, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]RoutineItem{}
	for rows.Next() {
		var it RoutineItem
		var routine int64
		var rmin, rmax, dur, rest sql.NullInt64
		var w, dist sql.NullFloat64
		if err := rows.Scan(&it.ID, &routine, &it.ExerciseID, &it.Position, &it.Sets, &rmin, &rmax, &w, &dur, &dist, &rest, &it.Notes); err != nil {
			return nil, err
		}
		it.RepsMin, it.RepsMax, it.DurationS, it.RestS = nInt(rmin), nInt(rmax), nInt(dur), nInt(rest)
		it.Weight, it.DistanceM = nFloat(w), nFloat(dist)
		it.Exercise = ex[it.ExerciseID]
		out[routine] = append(out[routine], it)
	}
	return out, rows.Err()
}

// GetRoutine returns one of a person's routines with its exercises.
func (s *Store) GetRoutine(ctx context.Context, uid, id int64) (*Routine, error) {
	r, err := scanRoutine(s.DB.QueryRowContext(ctx, `SELECT `+routineCols+` FROM routines WHERE user_id = ? AND id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	items, err := s.routineItems(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if items[id] != nil {
		r.Items = items[id]
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(date), ''), COUNT(1) FROM workouts WHERE user_id = ? AND routine_id = ? AND ended_at <> ''`, uid, id).Scan(&r.LastDone, &r.Times)
	return r, nil
}

// SaveRoutine creates (ID 0) or updates a routine and replaces its items.
// Every item's exercise must be the person's own (checked by the caller).
func (s *Store) SaveRoutine(ctx context.Context, r *Routine) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if r.ID == 0 {
		var sort int
		_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort), 0) + 1 FROM routines WHERE user_id = ?`, r.UserID).Scan(&sort)
		res, err := tx.ExecContext(ctx, `INSERT INTO routines (user_id, name, notes, outdoors, sort, source) VALUES (?, ?, ?, ?, ?, ?)`,
			r.UserID, r.Name, r.Notes, boolInt(r.Outdoors), sort, r.Source)
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE routines SET name = ?, notes = ?, outdoors = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
			r.Name, r.Notes, boolInt(r.Outdoors), Now(), r.UserID, r.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM routine_items WHERE user_id = ? AND routine_id = ?`, r.UserID, r.ID); err != nil {
			return err
		}
	}
	for i, it := range r.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO routine_items (user_id, routine_id, exercise_id, position, sets, reps_min, reps_max, weight, duration_s, distance_m, rest_s, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.UserID, r.ID, it.ExerciseID, i, it.Sets, pInt(it.RepsMin), pInt(it.RepsMax), pFloat(it.Weight),
			pInt(it.DurationS), pFloat(it.DistanceM), pInt(it.RestS), it.Notes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OrderRoutines saves the order of a person's routines.
func (s *Store) OrderRoutines(ctx context.Context, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE routines SET sort = ? WHERE user_id = ? AND id = ?`, i+1, uid, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// DeleteRoutine removes a routine (its workouts stay, without it).
func (s *Store) DeleteRoutine(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM routines WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPlan returns a person's week: the days that have a routine.
func (s *Store) GetPlan(ctx context.Context, uid int64) ([]PlanDay, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.weekday, p.routine_id, p.time, r.name FROM plan p JOIN routines r ON r.id = p.routine_id AND r.user_id = p.user_id
		WHERE p.user_id = ? ORDER BY p.weekday`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanDay{}
	for rows.Next() {
		var p PlanDay
		if err := rows.Scan(&p.Weekday, &p.RoutineID, &p.Time, &p.Routine); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetPlan replaces a person's week.
func (s *Store) SetPlan(ctx context.Context, uid int64, days []PlanDay) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM plan WHERE user_id = ?`, uid); err != nil {
		return err
	}
	for _, d := range days {
		if _, err := tx.ExecContext(ctx, `INSERT INTO plan (user_id, weekday, routine_id, time) VALUES (?, ?, ?, ?)`, uid, d.Weekday, d.RoutineID, d.Time); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PlanFor returns the planned session of a date, nil when it is a rest day.
func (s *Store) PlanFor(ctx context.Context, uid int64, date string) (*PlanDay, error) {
	t, err := ParseDate(date)
	if err != nil {
		return nil, err
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	plan, err := s.GetPlan(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, p := range plan {
		if p.Weekday == wd {
			return &p, nil
		}
	}
	return nil, nil
}

// PlanEvent is a planned session put on Pocket Calendar.
type PlanEvent struct {
	Date      string
	RoutineID int64
	Time      string
	EventID   int64
}

// PlanEvents returns the calendar events made for a person's plan.
func (s *Store) PlanEvents(ctx context.Context, uid int64) (map[string]PlanEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, routine_id, time, event_id FROM plan_events WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PlanEvent{}
	for rows.Next() {
		var e PlanEvent
		if err := rows.Scan(&e.Date, &e.RoutineID, &e.Time, &e.EventID); err != nil {
			return nil, err
		}
		out[e.Date] = e
	}
	return out, rows.Err()
}

// SavePlanEvent remembers (event 0: forgets) the event of a date.
func (s *Store) SavePlanEvent(ctx context.Context, uid int64, e PlanEvent) error {
	if e.EventID == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM plan_events WHERE user_id = ? AND date = ?`, uid, e.Date)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO plan_events (user_id, date, routine_id, time, event_id) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET routine_id = excluded.routine_id, time = excluded.time, event_id = excluded.event_id`, uid, e.Date, e.RoutineID, e.Time, e.EventID)
	return err
}

func nInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

func nFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

func pInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func pFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
