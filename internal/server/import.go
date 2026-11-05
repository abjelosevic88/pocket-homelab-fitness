package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// handleImport takes a Strong or Hevy CSV export, or a Pocket Fitness
// export (.json), as the request body. Importing the same file twice adds
// nothing: workouts are matched by their start time and name. Strong writes
// weights in the unit the app was set to: ?unit=lb when that was pounds.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too large")
		return
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	if t := bytes.TrimSpace(body); len(t) > 0 && t[0] == '{' {
		var doc exportDoc
		if err := json.Unmarshal(body, &doc); err != nil || doc.App != "pocket-fitness" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket Fitness export (.json)")
			return
		}
		res, err := s.importDoc(ctx, uid, doc)
		if err != nil {
			s.fail(w, err, "import")
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	lb := r.URL.Query().Get("unit") == "lb"
	workouts, format, err := parseCSV(body, lb)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.importWorkouts(ctx, uid, workouts, format)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	res["format"] = format
	writeJSON(w, http.StatusOK, res)
}

// csvWorkout is one workout read from a CSV export.
type csvWorkout struct {
	name, notes string
	start, end  time.Time
	exercises   []csvExercise
}

type csvExercise struct {
	name string
	sets []setIn
}

// parseCSV reads Strong's or Hevy's export (and Pocket Fitness's CSV,
// which has Hevy's columns).
func parseCSV(body []byte, lb bool) ([]*csvWorkout, string, error) {
	first := string(body)
	if i := strings.IndexByte(first, '\n'); i > 0 {
		first = first[:i]
	}
	cr := csv.NewReader(bytes.NewReader(body))
	if strings.Count(first, ";") > strings.Count(first, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, "", fmt.Errorf("that is not a CSV export with workouts")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, k string) string {
		if i, ok := col[k]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	has := func(k string) bool { _, ok := col[k]; return ok }
	format := ""
	switch {
	case has("exercise_title") && has("start_time"):
		format = "hevy"
	case has("exercise name") && has("date"):
		format = "strong"
	default:
		return nil, "", fmt.Errorf("unknown CSV: export your workouts from Strong (Settings → Export data) or Hevy (Settings → Export & import data)")
	}
	var list []*csvWorkout
	byKey := map[string]*csvWorkout{}
	for _, row := range rows[1:] {
		var key, name, exName, notes string
		var start, end time.Time
		var st setIn
		var ok bool
		switch format {
		case "hevy":
			name, exName, notes = get(row, "title"), get(row, "exercise_title"), get(row, "description")
			if start, ok = parseAnyTime(get(row, "start_time")); !ok {
				continue
			}
			end, _ = parseAnyTime(get(row, "end_time"))
			st.Type = map[string]string{"warmup": "warmup", "dropset": "drop", "drop": "drop", "failure": "failure"}[strings.ToLower(get(row, "set_type"))]
			if v, ok := csvNum(get(row, "weight_kg")); ok {
				st.Weight = &v
			} else if v, ok := csvNum(get(row, "weight_lbs")); ok {
				v *= 0.45359237
				st.Weight = &v
			}
			if v, ok := csvNum(get(row, "distance_km")); ok {
				v *= 1000
				st.DistanceM = &v
			} else if v, ok := csvNum(get(row, "distance_miles")); ok {
				v *= 1609.344
				st.DistanceM = &v
			}
			if v, ok := csvNum(get(row, "duration_seconds")); ok {
				d := int(v)
				st.DurationS = &d
			}
			if v, ok := csvNum(get(row, "rpe")); ok && v >= 1 && v <= 10 {
				st.RPE = &v
			}
			st.Note = get(row, "exercise_notes")
		case "strong":
			name, exName, notes = get(row, "workout name"), get(row, "exercise name"), get(row, "workout notes")
			order := strings.ToUpper(get(row, "set order"))
			if strings.Contains(order, "REST") {
				continue
			}
			if start, ok = parseAnyTime(get(row, "date")); !ok {
				continue
			}
			end = start.Add(strongDuration(get(row, "duration")))
			st.Type = map[string]string{"W": "warmup", "D": "drop", "F": "failure"}[order]
			if v, ok := csvNum(get(row, "weight")); ok {
				if lb {
					v *= 0.45359237
				}
				st.Weight = &v
			}
			if v, ok := csvNum(get(row, "distance")); ok && v > 0 {
				v *= 1000
				if lb {
					v *= 1.609344
				}
				st.DistanceM = &v
			}
			if v, ok := csvNum(get(row, "seconds")); ok && v > 0 {
				d := int(v)
				st.DurationS = &d
			}
			if v, ok := csvNum(get(row, "rpe")); ok && v >= 1 && v <= 10 {
				st.RPE = &v
			}
			st.Note = get(row, "notes")
		}
		if exName == "" {
			continue
		}
		if v, ok := csvNum(get(row, "reps")); ok {
			n := int(v)
			st.Reps = &n
		}
		if st.Weight != nil && *st.Weight == 0 {
			st.Weight = nil
		}
		key = start.UTC().Format(time.RFC3339) + "|" + name
		wk := byKey[key]
		if wk == nil {
			wk = &csvWorkout{name: name, notes: notes, start: start, end: end}
			byKey[key] = wk
			list = append(list, wk)
		}
		if n := len(wk.exercises); n == 0 || wk.exercises[n-1].name != exName {
			wk.exercises = append(wk.exercises, csvExercise{name: exName})
		}
		e := &wk.exercises[len(wk.exercises)-1]
		e.sets = append(e.sets, st)
	}
	if len(list) == 0 {
		return nil, "", fmt.Errorf("no workouts found in the file")
	}
	sort.Slice(list, func(a, b int) bool { return list[a].start.Before(list[b].start) })
	return list, format, nil
}

func csvNum(v string) (float64, bool) {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", "."))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// parseAnyTime reads the times exports write: ISO, "2023-01-15 18:30:00",
// Hevy's "15 Jan 2023, 18:30".
func parseAnyTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2 Jan 2006, 15:04", "2 Jan 2006 15:04", "Jan 2, 2006, 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var strongDur = regexp.MustCompile(`(\d+)\s*([hms])`)

// strongDuration reads Strong's "1h 5m" (or plain seconds).
func strongDuration(v string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return time.Duration(n) * time.Second
	}
	var d time.Duration
	for _, m := range strongDur.FindAllStringSubmatch(v, -1) {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "h":
			d += time.Duration(n) * time.Hour
		case "m":
			d += time.Duration(n) * time.Minute
		case "s":
			d += time.Duration(n) * time.Second
		}
	}
	return d
}

// importWorkouts saves workouts read from a file (source = the format, so
// a second import updates instead of adding).
func (s *Server) importWorkouts(ctx context.Context, uid int64, list []*csvWorkout, format string) (map[string]any, error) {
	res := map[string]any{"workouts": 0, "updated": 0, "sets": 0, "exercises_added": 0}
	before, _ := s.store.ListExercises(ctx, uid, true)
	for _, cw := range list {
		var exs []exerciseIn
		for _, e := range cw.exercises {
			exs = append(exs, exerciseIn{Name: e.name, Kind: guessKind(e.sets), Sets: e.sets})
		}
		mins := cw.end.Sub(cw.start).Minutes()
		wk, err := s.buildLogged(ctx, uid, cw.name, "", cw.start.Format(time.RFC3339), mins, exs)
		if err != nil {
			continue
		}
		wk.Bodyweight = nil
		wk.Notes, wk.Source, wk.SourceRef = cw.notes, format, cw.start.UTC().Format(time.RFC3339)+"|"+cw.name
		if old, err := s.store.WorkoutBySource(ctx, uid, wk.Source, wk.SourceRef); err == nil {
			wk.ID, wk.HealthRef = old.ID, old.HealthRef
			res["updated"] = res["updated"].(int) + 1
		} else {
			res["workouts"] = res["workouts"].(int) + 1
		}
		if err := s.store.SaveWorkout(ctx, wk); err != nil {
			return nil, err
		}
		res["sets"] = res["sets"].(int) + len(wk.Sets)
	}
	after, _ := s.store.ListExercises(ctx, uid, true)
	res["exercises_added"] = len(after) - len(before)
	return res, nil
}

// guessKind is what an exercise new to the person holds, from its sets.
func guessKind(sets []setIn) string {
	for _, st := range sets {
		switch {
		case st.DistanceM != nil && *st.DistanceM > 0:
			return "cardio"
		case st.Weight != nil && *st.Weight > 0:
			return "weight"
		}
	}
	for _, st := range sets {
		if st.Reps != nil {
			return "reps"
		}
		if st.DurationS != nil {
			return "time"
		}
	}
	return "weight"
}

// importDoc takes a Pocket Fitness export back: exercises by name, routines
// by name (ones you already have are kept), the plan when yours is empty,
// and workouts by their start time.
func (s *Server) importDoc(ctx context.Context, uid int64, doc exportDoc) (map[string]any, error) {
	res := map[string]any{"workouts": 0, "updated": 0, "routines": 0, "exercises_added": 0}
	if err := s.store.EnsureCatalog(ctx, uid); err != nil {
		return nil, err
	}
	before, _ := s.store.ListExercises(ctx, uid, true)
	exID := map[int64]int64{} // id in the file → id here
	for _, e := range doc.Exercises {
		got, err := s.store.ResolveExercise(ctx, uid, e.Name, e.Kind)
		if err != nil {
			continue
		}
		if got.Builtin == "" && got.Muscle == "other" && store.ValidMuscle(e.Muscle) {
			got.Muscle, got.Equipment = e.Muscle, e.Equipment
			if !store.ValidEquipment(got.Equipment) {
				got.Equipment = "other"
			}
			_ = s.store.UpdateExercise(ctx, got)
		}
		if e.Archived && !got.Archived {
			got.Archived = true
			_ = s.store.UpdateExercise(ctx, got)
		}
		exID[e.ID] = got.ID
	}
	have, _ := s.store.ListRoutines(ctx, uid)
	names := map[string]int64{}
	for _, r := range have {
		names[strings.ToLower(r.Name)] = r.ID
	}
	rtID := map[int64]int64{}
	for _, r := range doc.Routines {
		if id, ok := names[strings.ToLower(r.Name)]; ok {
			rtID[r.ID] = id
			continue
		}
		nr := &store.Routine{UserID: uid, Name: r.Name, Notes: r.Notes, Outdoors: r.Outdoors, Source: "import"}
		for _, it := range r.Items {
			if id := exID[it.ExerciseID]; id > 0 {
				it.ExerciseID = id
				it.Exercise = nil
				nr.Items = append(nr.Items, it)
			}
		}
		if nr.Name == "" {
			continue
		}
		if err := s.store.SaveRoutine(ctx, nr); err != nil {
			return nil, err
		}
		rtID[r.ID] = nr.ID
		res["routines"] = res["routines"].(int) + 1
	}
	if cur, _ := s.store.GetPlan(ctx, uid); len(cur) == 0 && len(doc.Plan) > 0 {
		var days []store.PlanDay
		for _, p := range doc.Plan {
			if id := rtID[p.RoutineID]; id > 0 && p.Weekday >= 1 && p.Weekday <= 7 {
				days = append(days, store.PlanDay{Weekday: p.Weekday, RoutineID: id, Time: p.Time})
			}
		}
		_ = s.store.SetPlan(ctx, uid, days)
	}
	for _, w := range doc.Workouts {
		if w.InProgress() || w.StartedAt == "" {
			continue
		}
		nw := w
		nw.ID, nw.UserID, nw.HealthRef, nw.HealthError, nw.HabitDone = 0, uid, "", "", ""
		nw.Source, nw.SourceRef = "import", w.StartedAt+"|"+w.Name
		nw.RoutineID = nil
		if w.RoutineID != nil {
			if id := rtID[*w.RoutineID]; id > 0 {
				nw.RoutineID = &id
			}
		}
		nw.Sets = nil
		for _, st := range w.Sets {
			if id := exID[st.ExerciseID]; id > 0 {
				st.ExerciseID = id
				st.PR = nil
				nw.Sets = append(nw.Sets, st)
			}
		}
		if old, err := s.store.WorkoutBySource(ctx, uid, nw.Source, nw.SourceRef); err == nil {
			nw.ID = old.ID
			res["updated"] = res["updated"].(int) + 1
		} else if dup := s.sameStart(ctx, uid, w.StartedAt); dup {
			res["updated"] = res["updated"].(int) + 1
			continue
		} else {
			res["workouts"] = res["workouts"].(int) + 1
		}
		if err := s.store.SaveWorkout(ctx, &nw); err != nil {
			return nil, err
		}
	}
	after, _ := s.store.ListExercises(ctx, uid, true)
	res["exercises_added"] = len(after) - len(before)
	return res, nil
}

// sameStart reports whether the person already has a workout starting then
// (the export of this very account imported back).
func (s *Server) sameStart(ctx context.Context, uid int64, started string) bool {
	var n int
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM workouts WHERE user_id = ? AND started_at = ?`, uid, started).Scan(&n)
	return n > 0
}
