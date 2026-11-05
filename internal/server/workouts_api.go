package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// ---- workouts ----
//
// A workout is started (from a routine or empty), filled in set by set while
// it runs — the page saves the whole workout as it changes — and finished.
// Finishing works out the records, sends it to Pocket Health, ticks the
// habit and tells the Core bus. A finished workout can also be logged in one
// go (a past session, Pocket AI, another app).

var setTypes = []string{"normal", "warmup", "drop", "failure"}

// setIn is one set as sent by the page or another app.
type setIn struct {
	ExerciseID int64    `json:"exercise_id"`
	Exercise   string   `json:"exercise"` // a name, when logging
	Position   int      `json:"position"`
	Type       string   `json:"type"`
	Weight     *float64 `json:"weight"`
	Reps       *int     `json:"reps"`
	DurationS  *int     `json:"duration_s"`
	DistanceM  *float64 `json:"distance_m"`
	RPE        *float64 `json:"rpe"`
	Done       *bool    `json:"done"`
	Note       string   `json:"note"`
	RestS      *int     `json:"rest_s"`
}

// exerciseIn is one exercise with its sets (POST /workouts/log).
type exerciseIn struct {
	ExerciseID int64   `json:"exercise_id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Sets       []setIn `json:"sets"`
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// cleanSet checks one set and turns it into a stored one.
func cleanSet(in setIn, ex *store.Exercise) (store.Set, error) {
	st := store.Set{ExerciseID: ex.ID, Type: in.Type, Note: strings.TrimSpace(in.Note), Done: true}
	if st.Type == "" {
		st.Type = "normal"
	}
	if !oneOf(st.Type, setTypes) {
		return st, fmt.Errorf("a set is normal, warmup, drop or failure")
	}
	if in.Done != nil {
		st.Done = *in.Done
	}
	if in.Weight != nil {
		if *in.Weight < 0 || *in.Weight > 2000 {
			return st, fmt.Errorf("a weight is 0 to 2000 kg")
		}
		st.Weight = ptr(round1(*in.Weight*100) / 100)
	}
	if in.Reps != nil {
		if *in.Reps < 0 || *in.Reps > 1000 {
			return st, fmt.Errorf("reps are 0 to 1000")
		}
		st.Reps = in.Reps
	}
	if in.DurationS != nil {
		if *in.DurationS < 0 || *in.DurationS > 86400 {
			return st, fmt.Errorf("a time is up to 24 hours")
		}
		st.DurationS = in.DurationS
	}
	if in.DistanceM != nil {
		if *in.DistanceM < 0 || *in.DistanceM > 1_000_000 {
			return st, fmt.Errorf("a distance is up to 1000 km")
		}
		st.DistanceM = ptr(round1(*in.DistanceM))
	}
	if in.RPE != nil {
		if *in.RPE < 1 || *in.RPE > 10 {
			return st, fmt.Errorf("how hard (RPE) is 1 to 10")
		}
		st.RPE = in.RPE
	}
	if in.RestS != nil && *in.RestS >= 0 && *in.RestS <= 3600 {
		st.RestS = in.RestS
	}
	if len(st.Note) > 500 {
		st.Note = st.Note[:500]
	}
	return st, nil
}

// cleanSets checks a whole list of sets against the person's exercises and
// numbers them per exercise in the order they came.
func (s *Server) cleanSets(ctx context.Context, uid int64, in []setIn) ([]store.Set, error) {
	if len(in) > 500 {
		return nil, fmt.Errorf("a workout has at most 500 sets")
	}
	ex, err := s.store.ExercisesByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]store.Set, 0, len(in))
	setNo := map[int]int{}
	for _, si := range in {
		e := ex[si.ExerciseID]
		if e == nil && si.Exercise != "" {
			if e, err = s.store.ResolveExercise(ctx, uid, si.Exercise, ""); err != nil {
				return nil, err
			}
			ex[e.ID] = e
		}
		if e == nil {
			return nil, store.ErrNotFound
		}
		st, err := cleanSet(si, e)
		if err != nil {
			return nil, err
		}
		st.Position = si.Position
		setNo[si.Position]++
		st.SetNo = setNo[si.Position]
		out = append(out, st)
	}
	return out, nil
}

// localDate is the local date of an RFC 3339 time.
func localDate(at string) string {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return store.Today()
	}
	return t.Local().Format("2006-01-02")
}

func parseTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	for _, f := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// workoutOut is a workout with what the page needs around it: the exercises
// it uses and, while it runs, what was done last time.
func (s *Server) workoutOut(ctx context.Context, uid int64, w *store.Workout) map[string]any {
	ex, _ := s.store.ExercisesByID(ctx, uid)
	used := map[string]*store.Exercise{}
	var ids []int64
	for _, st := range w.Sets {
		if e := ex[st.ExerciseID]; e != nil {
			if _, ok := used[itoa(e.ID)]; !ok {
				ids = append(ids, e.ID)
			}
			used[itoa(e.ID)] = e
		}
	}
	out := map[string]any{"workout": w, "exercises": used}
	prev, _ := s.store.Previous(ctx, uid, ids, w.StartedAt, w.ID)
	pm := map[string][]store.Set{}
	for k, v := range prev {
		pm[itoa(k)] = v
	}
	out["previous"] = pm
	if w.RoutineID != nil {
		if r, err := s.store.GetRoutine(ctx, uid, *w.RoutineID); err == nil {
			out["routine"] = map[string]any{"id": r.ID, "name": r.Name, "outdoors": r.Outdoors}
		}
	}
	return out
}

func (s *Server) handleListWorkouts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	if !validDate(q.Get("from")) || !validDate(q.Get("to")) {
		writeErr(w, http.StatusBadRequest, "dates are YYYY-MM-DD")
		return
	}
	list, err := s.store.ListWorkouts(ctx, actorID(ctx), store.WorkoutFilter{From: q.Get("from"), To: q.Get("to"), RoutineID: int64(queryInt(r, "routine")),
		ExerciseID: int64(queryInt(r, "exercise")), Finished: !queryBool(r, "all"), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")})
	if err != nil {
		s.fail(w, err, "workouts")
		return
	}
	if !queryBool(r, "sets") {
		for i := range list {
			list[i].Sets = nil
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	writeJSON(w, http.StatusOK, s.workoutOut(ctx, uid, wk))
}

func (s *Server) handleActiveWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.ActiveWorkout(ctx, uid)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"workout": nil})
		return
	}
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	writeJSON(w, http.StatusOK, s.workoutOut(ctx, uid, wk))
}

// handleStartWorkout starts a workout, empty or from a routine (its sets
// filled with what was done last time, else the routine's aim). One runs at
// a time: when one is running, that one comes back.
func (s *Server) handleStartWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		RoutineID int64  `json:"routine_id"`
		Name      string `json:"name"`
	}
	_ = decode(r, &in)
	if cur, err := s.store.ActiveWorkout(ctx, uid); err == nil {
		writeJSON(w, http.StatusOK, s.workoutOut(ctx, uid, cur))
		return
	}
	now := time.Now()
	wk := &store.Workout{UserID: uid, Name: strings.TrimSpace(in.Name), StartedAt: rfc(now), Date: now.Format("2006-01-02")}
	if in.RoutineID > 0 {
		rt, err := s.store.GetRoutine(ctx, uid, in.RoutineID)
		if err != nil {
			s.fail(w, err, "routine")
			return
		}
		wk.RoutineID = &rt.ID
		if wk.Name == "" {
			wk.Name = rt.Name
		}
		ids := make([]int64, 0, len(rt.Items))
		for _, it := range rt.Items {
			ids = append(ids, it.ExerciseID)
		}
		prev, _ := s.store.Previous(ctx, uid, ids, wk.StartedAt, 0)
		for pos, it := range rt.Items {
			last := prev[it.ExerciseID]
			var lastWork []store.Set
			for _, p := range last {
				if p.Type != "warmup" {
					lastWork = append(lastWork, p)
				}
			}
			n := it.Sets
			if n <= 0 {
				n = 3
			}
			for i := 0; i < n; i++ {
				st := store.Set{ExerciseID: it.ExerciseID, Position: pos, SetNo: i + 1, Type: "normal", RestS: it.RestS}
				switch {
				case len(lastWork) > 0:
					p := lastWork[min(i, len(lastWork)-1)]
					st.Weight, st.Reps, st.DurationS, st.DistanceM = p.Weight, p.Reps, p.DurationS, p.DistanceM
				default:
					st.Weight, st.DurationS, st.DistanceM = it.Weight, it.DurationS, it.DistanceM
					if it.RepsMax != nil {
						st.Reps = it.RepsMax
					} else {
						st.Reps = it.RepsMin
					}
				}
				if it.Weight != nil && st.Weight == nil {
					st.Weight = it.Weight
				}
				wk.Sets = append(wk.Sets, st)
			}
		}
	}
	if wk.Name == "" {
		wk.Name = defaultWorkoutName(now)
	}
	if err := s.store.SaveWorkout(ctx, wk); err != nil {
		s.fail(w, err, "start workout")
		return
	}
	got, err := s.store.GetWorkout(ctx, uid, wk.ID)
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	writeJSON(w, http.StatusCreated, s.workoutOut(ctx, uid, got))
}

// defaultWorkoutName is "Morning workout", "Evening workout"…
func defaultWorkoutName(t time.Time) string {
	switch h := t.Hour(); {
	case h < 5:
		return "Night workout"
	case h < 12:
		return "Morning workout"
	case h < 17:
		return "Afternoon workout"
	case h < 22:
		return "Evening workout"
	}
	return "Night workout"
}

// handleUpdateWorkout saves a workout as it is now: its details and the
// whole list of sets (sent again on every change). An app acting through a
// Pocket Core link may only change the workouts it logged itself.
func (s *Server) handleUpdateWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	if app := linkApp(ctx); app != "" && wk.Source != app {
		writeErr(w, http.StatusForbidden, "this workout was not logged by "+app)
		return
	}
	var in struct {
		Name      *string  `json:"name"`
		Notes     *string  `json:"notes"`
		Feel      *int     `json:"feel"`
		StartedAt *string  `json:"started_at"`
		EndedAt   *string  `json:"ended_at"`
		RoutineID *int64   `json:"routine_id"`
		Sets      *[]setIn `json:"sets"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); n != "" {
			wk.Name = n
		}
	}
	trimP(&wk.Notes, in.Notes)
	if in.Feel != nil {
		if *in.Feel < 0 || *in.Feel > 5 {
			writeErr(w, http.StatusBadRequest, "how it felt is 1 to 5")
			return
		}
		wk.Feel = *in.Feel
	}
	if in.RoutineID != nil {
		wk.RoutineID = nil
		if *in.RoutineID > 0 {
			if _, err := s.store.GetRoutine(ctx, uid, *in.RoutineID); err != nil {
				s.fail(w, err, "routine")
				return
			}
			wk.RoutineID = in.RoutineID
		}
	}
	if in.StartedAt != nil {
		t, ok := parseTime(*in.StartedAt)
		if !ok {
			writeErr(w, http.StatusBadRequest, "the start is a date and time")
			return
		}
		wk.StartedAt, wk.Date = rfc(t), t.Format("2006-01-02")
	}
	if in.EndedAt != nil && !wk.InProgress() {
		t, ok := parseTime(*in.EndedAt)
		st, _ := time.Parse(time.RFC3339Nano, wk.StartedAt)
		if !ok || !t.After(st) {
			writeErr(w, http.StatusBadRequest, "the end is after the start")
			return
		}
		wk.EndedAt, wk.DurationS = rfc(t), int(t.Sub(st).Seconds())
	} else if !wk.InProgress() && in.StartedAt != nil {
		st, _ := time.Parse(time.RFC3339Nano, wk.StartedAt)
		wk.EndedAt = rfc(st.Add(time.Duration(wk.DurationS) * time.Second))
	}
	if in.Sets != nil {
		sets, err := s.cleanSets(ctx, uid, *in.Sets)
		if err != nil {
			s.failInput(w, err)
			return
		}
		wk.Sets = sets
	}
	if err := s.store.SaveWorkout(ctx, wk); err != nil {
		s.fail(w, err, "save workout")
		return
	}
	got, err := s.store.GetWorkout(ctx, uid, wk.ID)
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	if !got.InProgress() && got.HealthRef != "" {
		// the copy in Pocket Health follows (same start time and app = updated)
		go s.sendToHealth(context.WithoutCancel(ctx), uid, got)
	}
	writeJSON(w, http.StatusOK, s.workoutOut(ctx, uid, got))
}

// failInput is a 404 for someone else's exercise or routine, else a 400.
func (s *Server) failInput(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeErr(w, http.StatusBadRequest, err.Error())
}

// handleFinishWorkout ends a running workout: sets that were not ticked are
// dropped, the totals and the new records worked out, and the other apps told.
func (s *Server) handleFinishWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	if !wk.InProgress() {
		writeErr(w, http.StatusConflict, "this workout is already finished")
		return
	}
	var in struct {
		EndedAt string `json:"ended_at"`
		Notes   string `json:"notes"`
		Feel    int    `json:"feel"`
	}
	_ = decode(r, &in)
	end := time.Now()
	if t, ok := parseTime(in.EndedAt); ok && in.EndedAt != "" {
		end = t
	}
	kept := wk.Sets[:0]
	for _, st := range wk.Sets {
		if st.Done {
			kept = append(kept, st)
		}
	}
	if len(kept) == 0 {
		writeErr(w, http.StatusBadRequest, "no set is ticked as done yet: tick the sets you did, or discard the workout")
		return
	}
	wk.Sets = kept
	start, _ := time.Parse(time.RFC3339Nano, wk.StartedAt)
	if !end.After(start) {
		end = start.Add(time.Minute)
	}
	wk.EndedAt, wk.DurationS = rfc(end), int(end.Sub(start).Seconds())
	if strings.TrimSpace(in.Notes) != "" {
		wk.Notes = strings.TrimSpace(in.Notes)
	}
	if in.Feel >= 1 && in.Feel <= 5 {
		wk.Feel = in.Feel
	}
	if bw := s.bodyweight(ctx, uid); bw > 0 {
		wk.Bodyweight = &bw
	}
	if err := s.store.SaveWorkout(ctx, wk); err != nil {
		s.fail(w, err, "finish workout")
		return
	}
	writeJSON(w, http.StatusOK, s.finished(ctx, uid, wk.ID))
}

// finished runs what follows a finished workout and says how it went: the
// records it set, the week so far, and what the other apps made of it.
func (s *Server) finished(ctx context.Context, uid, id int64) map[string]any {
	wk, err := s.store.GetWorkout(ctx, uid, id)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	links := s.afterFinish(ctx, uid, wk)
	wk, _ = s.store.GetWorkout(ctx, uid, id)
	out := s.workoutOut(ctx, uid, wk)
	st, _ := s.store.GetSettings(ctx, uid)
	out["records"] = workoutRecords(wk, out["exercises"].(map[string]*store.Exercise), st.WeightUnit)
	out["links"] = links
	wkv := s.week(ctx, uid, store.Today())
	out["week"] = map[string]any{"done": wkv.Done, "goal": wkv.Goal, "met": wkv.Done >= wkv.Goal && wkv.Goal > 0}
	return out
}

// newRecord is a set of a workout that beat everything before it.
type newRecord struct {
	ExerciseID int64    `json:"exercise_id"`
	Exercise   string   `json:"exercise"`
	Kinds      []string `json:"kinds"` // weight | e1rm | volume | reps | duration | distance
	Weight     float64  `json:"weight,omitempty"`
	Reps       int      `json:"reps,omitempty"`
	DurationS  int      `json:"duration_s,omitempty"`
	DistanceM  float64  `json:"distance_m,omitempty"`
	E1RM       float64  `json:"e1rm,omitempty"`
	Text       string   `json:"text"`
}

func recordOf(st store.Set, name, unit string) newRecord {
	nr := newRecord{ExerciseID: st.ExerciseID, Exercise: name, Kinds: st.PR, E1RM: round1(st.E1RM()), Text: recordText(name, st, st.PR, unit)}
	if st.Weight != nil {
		nr.Weight = *st.Weight
	}
	if st.Reps != nil {
		nr.Reps = *st.Reps
	}
	if st.DurationS != nil {
		nr.DurationS = *st.DurationS
	}
	if st.DistanceM != nil {
		nr.DistanceM = *st.DistanceM
	}
	return nr
}

func workoutRecords(wk *store.Workout, ex map[string]*store.Exercise, unit string) []newRecord {
	out := []newRecord{}
	for _, st := range wk.Sets {
		if len(st.PR) == 0 {
			continue
		}
		name := ""
		if e := ex[itoa(st.ExerciseID)]; e != nil {
			name = e.Name
		}
		out = append(out, recordOf(st, name, unit))
	}
	return out
}

// handleLogWorkout records a finished workout in one go: a past session
// typed in, a quick log Pocket AI understood, or another app's. Exercises
// may be given by name (matched to yours or the built-in list, else added).
// With a source_ref the same workout sent again updates the first one.
func (s *Server) handleLogWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Name        string       `json:"name"`
		Date        string       `json:"date"`
		StartedAt   string       `json:"started_at"`
		DurationMin float64      `json:"duration_min"`
		Notes       string       `json:"notes"`
		Feel        int          `json:"feel"`
		RoutineID   int64        `json:"routine_id"`
		SourceRef   string       `json:"source_ref"`
		Exercises   []exerciseIn `json:"exercises"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	wk, err := s.buildLogged(ctx, uid, in.Name, in.Date, in.StartedAt, in.DurationMin, in.Exercises)
	if err != nil {
		s.failInput(w, err)
		return
	}
	wk.Notes, wk.SourceRef = strings.TrimSpace(in.Notes), strings.TrimSpace(in.SourceRef)
	if in.Feel >= 1 && in.Feel <= 5 {
		wk.Feel = in.Feel
	}
	if in.RoutineID > 0 {
		if _, err := s.store.GetRoutine(ctx, uid, in.RoutineID); err != nil {
			s.fail(w, err, "routine")
			return
		}
		wk.RoutineID = &in.RoutineID
	}
	wk.Source = linkApp(ctx)
	if wk.Source == "" && wk.SourceRef != "" {
		wk.Source = "api"
	}
	status := http.StatusCreated
	if wk.SourceRef != "" {
		if old, err := s.store.WorkoutBySource(ctx, uid, wk.Source, wk.SourceRef); err == nil {
			wk.ID, wk.HealthRef, status = old.ID, old.HealthRef, http.StatusOK
		}
	}
	if err := s.store.SaveWorkout(ctx, wk); err != nil {
		s.fail(w, err, "log workout")
		return
	}
	writeJSON(w, status, s.finished(ctx, uid, wk.ID))
}

// buildLogged makes a finished workout from exercises with sets.
func (s *Server) buildLogged(ctx context.Context, uid int64, name, date, startedAt string, minutes float64, exs []exerciseIn) (*store.Workout, error) {
	if len(exs) == 0 {
		return nil, fmt.Errorf("a workout needs at least one exercise with a set")
	}
	var start time.Time
	switch {
	case startedAt != "":
		t, ok := parseTime(startedAt)
		if !ok {
			return nil, fmt.Errorf("started_at is a date and time")
		}
		start = t
	case date != "":
		d, err := time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return nil, fmt.Errorf("the date is YYYY-MM-DD")
		}
		start = d.Add(18 * time.Hour)
		if date == store.Today() && time.Now().Before(start) {
			start = time.Now().Add(-time.Hour).Truncate(time.Minute)
		}
	default:
		start = time.Now().Add(-time.Hour).Truncate(time.Minute)
	}
	if start.After(time.Now().Add(10 * time.Minute)) {
		return nil, fmt.Errorf("a logged workout cannot be in the future")
	}
	var flat []setIn
	nsets := 0
	for pos, e := range exs {
		if e.ExerciseID == 0 {
			if strings.TrimSpace(e.Name) == "" {
				return nil, fmt.Errorf("each exercise needs an exercise_id or a name")
			}
			ex, err := s.store.ResolveExercise(ctx, uid, e.Name, e.Kind)
			if err != nil {
				return nil, err
			}
			e.ExerciseID = ex.ID
		}
		if len(e.Sets) == 0 {
			e.Sets = []setIn{{}}
		}
		for _, si := range e.Sets {
			si.ExerciseID, si.Exercise, si.Position = e.ExerciseID, "", pos
			flat = append(flat, si)
			nsets++
		}
	}
	sets, err := s.cleanSets(ctx, uid, flat)
	if err != nil {
		return nil, err
	}
	if minutes <= 0 {
		minutes = math.Max(20, float64(nsets)*3)
	}
	if minutes > 24*60 {
		return nil, fmt.Errorf("a workout lasts at most a day")
	}
	end := start.Add(time.Duration(minutes * float64(time.Minute)))
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultWorkoutName(start)
	}
	wk := &store.Workout{UserID: uid, Name: name, StartedAt: rfc(start), EndedAt: rfc(end), Date: start.Format("2006-01-02"), DurationS: int(end.Sub(start).Seconds()), Sets: sets}
	if bw := s.bodyweight(ctx, uid); bw > 0 {
		wk.Bodyweight = &bw
	}
	return wk, nil
}

func (s *Server) handleDeleteWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	if app := linkApp(ctx); app != "" && wk.Source != app {
		writeErr(w, http.StatusForbidden, "this workout was not logged by "+app)
		return
	}
	if err := s.store.DeleteWorkout(ctx, uid, wk.ID); err != nil {
		s.fail(w, err, "delete workout")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResendWorkout tells the other apps about a finished workout again
// (after switching a link on, or when Pocket Health was down).
func (s *Server) handleResendWorkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	if wk.InProgress() {
		writeErr(w, http.StatusConflict, "finish the workout first")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": s.afterFinish(ctx, uid, wk)})
}

// handleWorkoutToRoutine saves a workout's exercises as a routine (the sets
// it had, the reps and weights done as the aim).
func (s *Server) handleWorkoutToRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	wk, err := s.store.GetWorkout(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "workout")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	_ = decode(r, &in)
	rt := &store.Routine{UserID: uid, Name: strings.TrimSpace(in.Name)}
	if rt.Name == "" {
		rt.Name = wk.Name
	}
	byPos := map[int]*store.RoutineItem{}
	var order []int
	for _, st := range wk.Sets {
		if st.Type == "warmup" {
			continue
		}
		it := byPos[st.Position]
		if it == nil {
			it = &store.RoutineItem{ExerciseID: st.ExerciseID, RestS: st.RestS}
			byPos[st.Position] = it
			order = append(order, st.Position)
		}
		it.Sets++
		if st.Reps != nil && (it.RepsMax == nil || *st.Reps > *it.RepsMax) {
			it.RepsMax = st.Reps
		}
		if st.Weight != nil && (it.Weight == nil || *st.Weight > *it.Weight) {
			it.Weight = st.Weight
		}
		if st.DurationS != nil {
			it.DurationS = st.DurationS
		}
		if st.DistanceM != nil {
			it.DistanceM = st.DistanceM
		}
	}
	sort.Ints(order)
	for _, p := range order {
		rt.Items = append(rt.Items, *byPos[p])
	}
	if len(rt.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "this workout has no exercises to keep")
		return
	}
	if err := s.store.SaveRoutine(ctx, rt); err != nil {
		s.fail(w, err, "save routine")
		return
	}
	got, _ := s.store.GetRoutine(ctx, uid, rt.ID)
	writeJSON(w, http.StatusCreated, got)
}

// finishStale finishes workouts left running for more than a day (or drops
// them when nothing was ticked).
func (s *Server) finishStale(ctx context.Context, uid int64) {
	wk, err := s.store.ActiveWorkout(ctx, uid)
	if err != nil {
		return
	}
	start, err := time.Parse(time.RFC3339Nano, wk.StartedAt)
	if err != nil || time.Since(start) < 24*time.Hour {
		return
	}
	kept := wk.Sets[:0]
	for _, st := range wk.Sets {
		if st.Done {
			kept = append(kept, st)
		}
	}
	if len(kept) == 0 {
		_ = s.store.DeleteWorkout(ctx, uid, wk.ID)
		return
	}
	wk.Sets = kept
	d := time.Duration(math.Min(120, math.Max(20, float64(len(kept))*3))) * time.Minute
	wk.EndedAt, wk.DurationS = rfc(start.Add(d)), int(d.Seconds())
	if err := s.store.SaveWorkout(ctx, wk); err == nil {
		s.log.Info("finished a workout left running", "user", uid, "workout", wk.ID)
		s.afterFinish(ctx, uid, wk)
	}
}
