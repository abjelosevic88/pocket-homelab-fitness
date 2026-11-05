package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// ---- the week, today, a day, progress and records ----

// weekItem is one workout of a day: one of yours, or one from Pocket Health.
type weekItem struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"` // workout | running | cycling | … (Health's kinds)
	Minutes   int     `json:"minutes"`
	DistanceM float64 `json:"distance_m,omitempty"`
	Sets      int     `json:"sets,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Records   int     `json:"records,omitempty"`
	Health    bool    `json:"health,omitempty"`
}

type weekDay struct {
	Date     string         `json:"date"`
	Items    []weekItem     `json:"items"`
	Planned  *store.PlanDay `json:"planned"`
	Upcoming bool           `json:"upcoming"`
}

type weekView struct {
	Start  string    `json:"start"`
	End    string    `json:"end"`
	Days   []weekDay `json:"days"`
	Done   int       `json:"done"`
	Goal   int       `json:"goal"`
	Health bool      `json:"health"` // Pocket Health's workouts are in it
}

// week is the week a date is in: each day's workouts (with Pocket Health's
// when counted), what was planned, and how many of the goal are done.
func (s *Server) week(ctx context.Context, uid int64, date string) weekView {
	st, _ := s.store.GetSettings(ctx, uid)
	start := store.WeekOf(date, st.WeekStart)
	end := store.AddDays(start, 6)
	v := weekView{Start: start, End: end, Goal: st.WeekGoal}
	own, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{From: start, To: end, Finished: true, Limit: 200})
	hw := s.healthWorkouts(ctx, uid, start, end)
	v.Health = hw != nil
	byDay := map[string][]weekItem{}
	for _, w := range own {
		byDay[w.Date] = append(byDay[w.Date], weekItem{ID: w.ID, Name: w.Name, Kind: "workout", Minutes: w.DurationS / 60, Sets: w.SetCount, Volume: w.Volume, Records: w.Records, DistanceM: w.DistanceM})
		v.Done++
	}
	for _, h := range hw {
		byDay[h.Date] = append(byDay[h.Date], healthItem(h))
		v.Done++
	}
	plan, _ := s.store.GetPlan(ctx, uid)
	today := store.Today()
	for i := 0; i < 7; i++ {
		d := store.AddDays(start, i)
		t, _ := store.ParseDate(d)
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		day := weekDay{Date: d, Items: byDay[d], Upcoming: d > today}
		if day.Items == nil {
			day.Items = []weekItem{}
		}
		for _, p := range plan {
			if p.Weekday == wd {
				p := p
				day.Planned = &p
			}
		}
		v.Days = append(v.Days, day)
	}
	return v
}

func healthItem(h pocket.HealthWorkout) weekItem {
	name := h.Name
	if name == "" {
		name = strings.ToUpper(h.Kind[:1]) + strings.ReplaceAll(h.Kind[1:], "_", " ")
	}
	return weekItem{ID: h.ID, Name: name, Kind: h.Kind, Minutes: h.DurationS / 60, DistanceM: h.DistanceM, Health: true}
}

// streak is how many weeks in a row, up to last week, met the goal (this
// week counts once it is met).
func (s *Server) streak(ctx context.Context, uid int64) int {
	st, _ := s.store.GetSettings(ctx, uid)
	if st.WeekGoal <= 0 {
		return 0
	}
	start := store.WeekOf(store.Today(), st.WeekStart)
	from := store.AddDays(start, -7*52)
	days, _ := s.store.WorkoutDays(ctx, uid, from, store.AddDays(start, 6))
	count := map[string]int{}
	for d, dc := range days {
		count[store.WeekOf(d, st.WeekStart)] += dc.N
	}
	for _, h := range s.healthWorkouts(ctx, uid, from, store.AddDays(start, 6)) {
		count[store.WeekOf(h.Date, st.WeekStart)]++
	}
	n := 0
	if count[start] >= st.WeekGoal {
		n++
	}
	for wk := store.AddDays(start, -7); wk >= from; wk = store.AddDays(wk, -7) {
		if count[wk] < st.WeekGoal {
			break
		}
		n++
	}
	return n
}

// recentRecords are the latest sets that set a record (newest first).
func (s *Server) recentRecords(ctx context.Context, uid int64, limit int) []map[string]any {
	prs, _ := s.store.PRs(ctx, uid)
	if len(prs) == 0 {
		return []map[string]any{}
	}
	st, _ := s.store.GetSettings(ctx, uid)
	list, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{Finished: true, Limit: 60, From: store.AddDays(store.Today(), -120)})
	ex, _ := s.store.ExercisesByID(ctx, uid)
	out := []map[string]any{}
	for _, w := range list {
		seen := map[int64]bool{} // one record per exercise and workout: its best set
		for _, sx := range w.Sets {
			if len(sx.PR) == 0 || seen[sx.ExerciseID] {
				continue
			}
			seen[sx.ExerciseID] = true
			name := ""
			if e := ex[sx.ExerciseID]; e != nil {
				name = e.Name
			}
			r := recordOf(sx, name, st.WeightUnit)
			out = append(out, map[string]any{"exercise_id": r.ExerciseID, "exercise": r.Exercise, "kinds": r.Kinds, "weight": r.Weight, "reps": r.Reps,
				"duration_s": r.DurationS, "distance_m": r.DistanceM, "e1rm": r.E1RM, "workout_id": w.ID, "date": w.Date, "text": r.Text})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// todayPlan is today's planned routine with its exercises, nil on a rest day.
func (s *Server) todayPlan(ctx context.Context, uid int64, date string) map[string]any {
	p, _ := s.store.PlanFor(ctx, uid, date)
	if p == nil {
		return nil
	}
	r, err := s.store.GetRoutine(ctx, uid, p.RoutineID)
	if err != nil {
		return nil
	}
	st, _ := s.store.GetSettings(ctx, uid)
	out := map[string]any{"routine": r, "time": p.Time, "minutes": routineMinutes(r, st.RestS), "done": s.doneOn(ctx, uid, date)}
	if r.Outdoors {
		if wd := s.weatherFor(ctx, uid, date); wd != nil {
			out["weather"] = wd
		}
	}
	return out
}

// handleOverview is the Today page: a workout in progress, today's plan,
// the week, the latest workouts and records.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	today := store.Today()
	st, _ := s.store.GetSettings(ctx, uid)
	out := map[string]any{"date": today, "active": nil}
	if a, err := s.store.ActiveWorkout(ctx, uid); err == nil {
		a.Sets = nil
		out["active"] = a
	}
	out["today"] = s.todayPlan(ctx, uid, today)
	out["week"] = s.week(ctx, uid, today)
	recent, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{Finished: true, Limit: 5})
	for i := range recent {
		recent[i].Sets = nil
	}
	out["recent"] = recent
	out["records"] = s.recentRecords(ctx, uid, 5)
	out["streak"] = s.streak(ctx, uid)
	routines, _ := s.store.ListRoutines(ctx, uid)
	out["routines"] = routines
	if pts := s.weightSeries(ctx, uid, 30); len(pts) > 0 {
		out["weight"] = pts[len(pts)-1]
	}
	out["playlist"] = nil
	if u := s.playlistURL(ctx, uid, st); u != "" {
		out["playlist"] = map[string]any{"name": st.PlaylistName, "url": u}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := r.URL.Query().Get("date")
	if d == "" {
		d = store.Today()
	}
	if !validDate(d) {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	writeJSON(w, http.StatusOK, s.week(ctx, actorID(ctx), d))
}

// summaryText says the week in one sentence (Pocket AI, Pocket Day, Homepage).
func summaryText(wv weekView, last *store.Workout, plan map[string]any) string {
	var parts []string
	if wv.Goal > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d workouts this week.", wv.Done, wv.Goal))
	} else {
		parts = append(parts, fmt.Sprintf("%d %s this week.", wv.Done, plural(wv.Done, "workout", "workouts")))
	}
	if plan != nil {
		if r, ok := plan["routine"].(*store.Routine); ok {
			t := ""
			if tm, _ := plan["time"].(string); tm != "" {
				t = " at " + tm
			}
			if done, _ := plan["done"].(bool); done {
				parts = append(parts, "Today's "+r.Name+" is done.")
			} else {
				parts = append(parts, "Today: "+r.Name+t+".")
			}
		}
	}
	if last != nil {
		parts = append(parts, fmt.Sprintf("Last workout: %s on %s.", last.Name, dayWord(last.Date)))
	}
	return strings.Join(parts, " ")
}

// dayWord is "today", "yesterday", "Monday" (this week) or "3 Nov".
func dayWord(d string) string {
	today := store.Today()
	switch d {
	case today:
		return "today"
	case store.AddDays(today, -1):
		return "yesterday"
	}
	t, err := store.ParseDate(d)
	if err != nil {
		return d
	}
	if d > store.AddDays(today, -7) {
		return t.Weekday().String()
	}
	return t.Format("2 Jan")
}

// handleSummary is the short version for other apps and Pocket AI.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	today := store.Today()
	wv := s.week(ctx, uid, today)
	plan := s.todayPlan(ctx, uid, today)
	var last *store.Workout
	if l, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{Finished: true, Limit: 1}); len(l) > 0 {
		last = &l[0]
		last.Sets = nil
	}
	out := map[string]any{"date": today, "week_done": wv.Done, "week_goal": wv.Goal, "week_start": wv.Start, "streak_weeks": s.streak(ctx, uid),
		"last_workout": last, "records": s.recentRecords(ctx, uid, 3), "text": summaryText(wv, last, plan), "url": s.appLink("/")}
	out["today"] = nil
	if plan != nil {
		rt := plan["routine"].(*store.Routine)
		out["today"] = map[string]any{"routine": rt.Name, "routine_id": rt.ID, "time": plan["time"], "exercises": len(rt.Items), "done": plan["done"], "minutes": plan["minutes"]}
	}
	if a, err := s.store.ActiveWorkout(ctx, uid); err == nil {
		out["in_progress"] = map[string]any{"id": a.ID, "name": a.Name, "started_at": a.StartedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDay is one date: its workouts with each exercise's sets in words,
// Pocket Health's other workouts and what was planned (Pocket Day's journal).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chi.URLParam(r, "date")
	if _, err := store.ParseDate(date); err != nil {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	list, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{From: date, To: date, Finished: true, Limit: 20})
	ex, _ := s.store.ExercisesByID(ctx, uid)
	var workouts []map[string]any
	var lines []string
	for i := range list {
		wk := &list[i]
		sort.SliceStable(wk.Sets, func(a, b int) bool { return wk.Sets[a].Position < wk.Sets[b].Position })
		var exs []map[string]any
		pos := -1
		var cur map[string]any
		var sets []string
		flush := func() {
			if cur != nil {
				cur["sets"] = strings.Join(sets, ", ")
				exs = append(exs, cur)
			}
		}
		for _, sx := range wk.Sets {
			if sx.Position != pos {
				flush()
				pos = sx.Position
				name := ""
				if e := ex[sx.ExerciseID]; e != nil {
					name = e.Name
				}
				cur, sets = map[string]any{"exercise_id": sx.ExerciseID, "exercise": name, "records": 0}, nil
			}
			if sx.Type == "warmup" {
				continue
			}
			sets = append(sets, setWords(sx, st.WeightUnit))
			if len(sx.PR) > 0 {
				cur["records"] = cur["records"].(int) + 1
			}
		}
		flush()
		workouts = append(workouts, map[string]any{"id": wk.ID, "name": wk.Name, "started_at": wk.StartedAt, "duration_min": wk.DurationS / 60,
			"sets": wk.SetCount, "volume_kg": wk.Volume, "records": wk.Records, "feel": wk.Feel, "notes": wk.Notes, "exercises": exs, "url": s.appLink("/workouts/" + itoa(wk.ID))})
		lines = append(lines, fmt.Sprintf("%s (%d min, %d sets)", wk.Name, wk.DurationS/60, wk.SetCount))
	}
	if workouts == nil {
		workouts = []map[string]any{}
	}
	other := []weekItem{}
	for _, h := range s.healthWorkouts(ctx, uid, date, date) {
		other = append(other, healthItem(h))
		lines = append(lines, fmt.Sprintf("%s (%d min)", healthItem(h).Name, h.DurationS/60))
	}
	out := map[string]any{"date": date, "workouts": workouts, "other": other, "planned": nil}
	if p, _ := s.store.PlanFor(ctx, uid, date); p != nil {
		out["planned"] = p
	}
	text := "No workout."
	if len(lines) > 0 {
		text = strings.Join(lines, "; ") + "."
	} else if p, ok := out["planned"].(*store.PlanDay); ok && p != nil && date >= store.Today() {
		text = "Planned: " + p.Routine + "."
	}
	out["text"] = text
	writeJSON(w, http.StatusOK, out)
}

// setWords is a set in words: "80 kg × 8", "12 reps", "1:30", "5.2 km in 28:10".
func setWords(st store.Set, unit string) string {
	w := func(kg float64) string {
		if unit == "lb" {
			return trimNum(kg*2.20462) + " lb"
		}
		return trimNum(kg) + " kg"
	}
	switch {
	case st.DistanceM != nil && *st.DistanceM > 0:
		s := trimNum(*st.DistanceM/1000) + " km"
		if st.DurationS != nil && *st.DurationS > 0 {
			s += " in " + clockDur(*st.DurationS)
		}
		return s
	case st.Weight != nil && *st.Weight > 0 && st.Reps != nil:
		return fmt.Sprintf("%s × %d", w(*st.Weight), *st.Reps)
	case st.Reps != nil:
		return fmt.Sprintf("%d reps", *st.Reps)
	case st.DurationS != nil:
		return clockDur(*st.DurationS)
	}
	return "a set"
}

// handleProgress is the Progress page: weeks, muscles, weight, records.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	weeks := queryInt(r, "weeks")
	if weeks <= 0 || weeks > 104 {
		weeks = 12
	}
	today := store.Today()
	start := store.WeekOf(today, st.WeekStart)
	from := store.AddDays(start, -7*(weeks-1))
	vol, _ := s.store.WeekVolume(ctx, uid, from, store.AddDays(start, 6), st.WeekStart)
	days, _ := s.store.WorkoutDays(ctx, uid, from, store.AddDays(start, 6))
	hw := s.healthWorkouts(ctx, uid, from, store.AddDays(start, 6))
	type weekRow struct {
		Start    string  `json:"start"`
		Workouts int     `json:"workouts"`
		Health   int     `json:"health"`
		Volume   float64 `json:"volume"`
		Sets     int     `json:"sets"`
		Minutes  int     `json:"minutes"`
	}
	rows := map[string]*weekRow{}
	var list []*weekRow
	for wk := from; wk <= start; wk = store.AddDays(wk, 7) {
		rw := &weekRow{Start: wk, Volume: round1(vol[wk][0]), Sets: int(vol[wk][1])}
		rows[wk] = rw
		list = append(list, rw)
	}
	for d, dc := range days {
		if rw := rows[store.WeekOf(d, st.WeekStart)]; rw != nil {
			rw.Workouts += dc.N
			rw.Minutes += dc.Minutes
		}
	}
	for _, h := range hw {
		if rw := rows[store.WeekOf(h.Date, st.WeekStart)]; rw != nil {
			rw.Health++
			rw.Minutes += h.DurationS / 60
		}
	}
	ms, _ := s.store.MuscleSets(ctx, uid, store.AddDays(today, -29), today)
	type muscleRow struct {
		Muscle string  `json:"muscle"`
		Sets   float64 `json:"sets"`
	}
	muscles := []muscleRow{}
	for _, m := range store.Muscles {
		if v := ms[m]; v > 0 {
			muscles = append(muscles, muscleRow{m, round1(v)})
		}
	}
	sort.SliceStable(muscles, func(a, b int) bool { return muscles[a].Sets > muscles[b].Sets })
	var total struct {
		Workouts int     `json:"workouts"`
		Hours    float64 `json:"hours"`
		Volume   float64 `json:"volume"`
		Since    string  `json:"since"`
	}
	var secs int
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(SUM(duration_s), 0), COALESCE(MIN(date), '') FROM workouts WHERE user_id = ? AND ended_at <> ''`, uid).Scan(&total.Workouts, &secs, &total.Since)
	_ = s.store.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(weight * reps), 0) FROM workout_sets WHERE user_id = ? AND done = 1 AND type <> 'warmup' AND weight IS NOT NULL AND reps IS NOT NULL`, uid).Scan(&total.Volume)
	total.Hours, total.Volume = round1(float64(secs)/3600), round1(total.Volume)
	out := map[string]any{"weeks": list, "goal": st.WeekGoal, "muscles": muscles, "totals": total, "streak": s.streak(ctx, uid), "records": s.recentRecords(ctx, uid, 12), "health": hw != nil}
	out["weight"] = s.weightSeries(ctx, uid, 180)
	writeJSON(w, http.StatusOK, out)
}

// handleRecords lists the best at every exercise (?exercise= for one).
func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ex := int64(queryInt(r, "exercise"))
	if ex > 0 {
		if _, err := s.store.GetExercise(ctx, uid, ex); err != nil {
			s.fail(w, err, "exercise")
			return
		}
	}
	list, err := s.store.Records(ctx, uid, ex)
	if err != nil {
		s.fail(w, err, "records")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": list, "recent": s.recentRecords(ctx, uid, 20)})
}
