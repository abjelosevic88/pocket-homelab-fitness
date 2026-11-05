package store

import (
	"context"
	"fmt"
	"strings"
)

const exerciseCols = `id, user_id, name, builtin, muscle, secondary, equipment, kind, notes, archived, created_at, updated_at`

func scanExercise(sc interface{ Scan(...any) error }) (*Exercise, error) {
	var e Exercise
	var arch int
	if err := sc.Scan(&e.ID, &e.UserID, &e.Name, &e.Builtin, &e.Muscle, &e.Secondary, &e.Equipment, &e.Kind, &e.Notes, &arch, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Archived = arch == 1
	return &e, nil
}

// EnsureCatalog copies the built-in exercises into a person's account the
// first time (and any added to the list since), so they can be renamed or
// hidden like their own.
func (s *Store) EnsureCatalog(ctx context.Context, uid int64) error {
	key := fmt.Sprintf("catalog:%d:%d", uid, len(Catalog))
	if _, _, ok := s.CacheGet(ctx, key); ok {
		return nil
	}
	have := map[string]bool{}
	names := map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, `SELECT builtin, name FROM exercises WHERE user_id = ?`, uid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b, n string
		if err := rows.Scan(&b, &n); err != nil {
			rows.Close()
			return err
		}
		have[b] = true
		names[strings.ToLower(n)] = true
	}
	rows.Close()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, c := range Catalog {
		if have[c.Key] || names[strings.ToLower(c.Name)] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO exercises (user_id, name, builtin, muscle, secondary, equipment, kind) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uid, c.Name, c.Key, c.Muscle, c.Secondary, c.Equipment, c.Kind); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.CachePut(ctx, key, "1")
}

// ListExercises lists a person's exercises with how often and when they did them.
func (s *Store) ListExercises(ctx context.Context, uid int64, archived bool) ([]Exercise, error) {
	if err := s.EnsureCatalog(ctx, uid); err != nil {
		return nil, err
	}
	q := `SELECT e.id, e.user_id, e.name, e.builtin, e.muscle, e.secondary, e.equipment, e.kind, e.notes, e.archived, e.created_at, e.updated_at,
		COALESCE(x.n, 0), COALESCE(x.last, '')
		FROM exercises e LEFT JOIN (
			SELECT ws.exercise_id, COUNT(1) AS n, MAX(w.date) AS last FROM workout_sets ws JOIN workouts w ON w.id = ws.workout_id
			WHERE ws.user_id = ? AND ws.done = 1 GROUP BY ws.exercise_id) x ON x.exercise_id = e.id
		WHERE e.user_id = ?`
	if !archived {
		q += ` AND e.archived = 0`
	}
	q += ` ORDER BY e.name COLLATE NOCASE`
	rows, err := s.DB.QueryContext(ctx, q, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Exercise{}
	for rows.Next() {
		var e Exercise
		var arch int
		if err := rows.Scan(&e.ID, &e.UserID, &e.Name, &e.Builtin, &e.Muscle, &e.Secondary, &e.Equipment, &e.Kind, &e.Notes, &arch, &e.CreatedAt, &e.UpdatedAt, &e.Sets, &e.LastDone); err != nil {
			return nil, err
		}
		e.Archived = arch == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetExercise returns one of a person's exercises.
func (s *Store) GetExercise(ctx context.Context, uid, id int64) (*Exercise, error) {
	e, err := scanExercise(s.DB.QueryRowContext(ctx, `SELECT `+exerciseCols+` FROM exercises WHERE user_id = ? AND id = ?`, uid, id))
	return e, notFound(err)
}

// ExercisesByID returns a person's exercises by id (unknown ids are left out).
func (s *Store) ExercisesByID(ctx context.Context, uid int64) (map[int64]*Exercise, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+exerciseCols+` FROM exercises WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]*Exercise{}
	for rows.Next() {
		e, err := scanExercise(rows)
		if err != nil {
			return nil, err
		}
		out[e.ID] = e
	}
	return out, rows.Err()
}

// CreateExercise adds a person's own exercise.
func (s *Store) CreateExercise(ctx context.Context, e *Exercise) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO exercises (user_id, name, builtin, muscle, secondary, equipment, kind, notes) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Name, e.Builtin, e.Muscle, e.Secondary, e.Equipment, e.Kind, e.Notes)
	if err != nil {
		return conflict(err)
	}
	e.ID, _ = res.LastInsertId()
	got, err := s.GetExercise(ctx, e.UserID, e.ID)
	if err == nil {
		*e = *got
	}
	return err
}

// UpdateExercise saves an exercise.
func (s *Store) UpdateExercise(ctx context.Context, e *Exercise) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE exercises SET name = ?, muscle = ?, secondary = ?, equipment = ?, kind = ?, notes = ?, archived = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		e.Name, e.Muscle, e.Secondary, e.Equipment, e.Kind, e.Notes, boolInt(e.Archived), Now(), e.UserID, e.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExercise removes an exercise that was never done or hides one that
// was (its history stays). It reports whether it was only hidden.
func (s *Store) DeleteExercise(ctx context.Context, uid, id int64) (bool, error) {
	if _, err := s.GetExercise(ctx, uid, id); err != nil {
		return false, err
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM workout_sets WHERE user_id = ? AND exercise_id = ?`, uid, id).Scan(&n)
	if n > 0 {
		_, err := s.DB.ExecContext(ctx, `UPDATE exercises SET archived = 1, updated_at = ? WHERE user_id = ? AND id = ?`, Now(), uid, id)
		return true, err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM exercises WHERE user_id = ? AND id = ?`, uid, id)
	return false, err
}

// ResolveExercise finds the exercise a name means for a person — theirs by
// name, a built-in one by name or alias — or adds it as their own (kind is
// the guess for a new one). Hidden ones are brought back.
func (s *Store) ResolveExercise(ctx context.Context, uid int64, name, kind string) (*Exercise, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("an exercise needs a name")
	}
	if err := s.EnsureCatalog(ctx, uid); err != nil {
		return nil, err
	}
	find := func(q string, args ...any) *Exercise {
		e, err := scanExercise(s.DB.QueryRowContext(ctx, `SELECT `+exerciseCols+` FROM exercises WHERE user_id = ? AND `+q+` ORDER BY archived LIMIT 1`, append([]any{uid}, args...)...))
		if err != nil {
			return nil
		}
		return e
	}
	e := find(`name = ? COLLATE NOCASE`, name)
	if e == nil {
		if c, ok := CatalogMatch(name); ok {
			if e = find(`builtin = ?`, c.Key); e == nil {
				e = find(`name = ? COLLATE NOCASE`, c.Name)
			}
		}
	}
	if e == nil {
		// theirs, matched loosely ("pull-ups" = "Pull Ups")
		n := NormName(name)
		all, _ := s.ListExercises(ctx, uid, true)
		for i := range all {
			if NormName(all[i].Name) == n {
				e = &all[i]
				break
			}
		}
	}
	if e != nil {
		if e.Archived {
			e.Archived = false
			_ = s.UpdateExercise(ctx, e)
		}
		return e, nil
	}
	if !oneOfS(kind, Kinds) {
		kind = "weight"
	}
	ne := &Exercise{UserID: uid, Name: name, Muscle: "other", Equipment: "other", Kind: kind}
	if kind == "cardio" {
		ne.Muscle, ne.Equipment = "cardio", "cardio"
	}
	if err := s.CreateExercise(ctx, ne); err != nil {
		return nil, err
	}
	return ne, nil
}

func oneOfS(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ValidMuscle, ValidEquipment and ValidKind check the fixed lists.
func ValidMuscle(v string) bool    { return oneOfS(v, Muscles) }
func ValidEquipment(v string) bool { return oneOfS(v, Equipment) }
func ValidKind(v string) bool      { return oneOfS(v, Kinds) }
