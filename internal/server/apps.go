package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// ---- what Fitness does in the other Pocket apps ----
//
// Pocket Health: the runs, rides and walks from the watch count toward the
// week, the weight comes from there, and each finished workout is recorded
// there. Pocket Habits: the chosen habit is ticked on a training day.
// Pocket Calendar: planned sessions for the next two weeks. Pocket Weather:
// the day's weather for an outdoor session. Pocket Music: a playlist to
// train to. Every one of them is optional and switched per person.

// linkResult is what one app made of a finished workout, for the page.
type linkResult struct {
	State  string `json:"state"` // done | off | error
	Detail string `json:"detail,omitempty"`
}

// afterFinish runs what follows a finished workout: Pocket Health, Pocket
// Habits and the Core bus (the workout, each record, the week's goal met).
func (s *Server) afterFinish(ctx context.Context, uid int64, wk *store.Workout) map[string]linkResult {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	st, _ := s.store.GetSettings(ctx, uid)
	out := map[string]linkResult{}

	healthRef, healthErr := wk.HealthRef, ""
	switch {
	case !st.HealthSend:
		out["health"] = linkResult{State: "off", Detail: "switched off in Settings"}
	default:
		ref, err := s.sendToHealth(ctx, uid, wk)
		switch {
		case err == nil:
			healthRef = ref
			out["health"] = linkResult{State: "done"}
		case isNotConnected(err):
			out["health"] = linkResult{State: "off", Detail: err.Error()}
		default:
			healthErr = err.Error()
			out["health"] = linkResult{State: "error", Detail: healthErr}
		}
	}

	habitDone := wk.HabitDone
	switch {
	case st.HabitID == 0:
		out["habit"] = linkResult{State: "off", Detail: "no habit chosen in Settings"}
	default:
		err := s.tickHabit(ctx, uid, st, wk.Date)
		switch {
		case err == nil:
			habitDone = wk.Date
			out["habit"] = linkResult{State: "done", Detail: st.HabitName}
		case isNotConnected(err):
			out["habit"] = linkResult{State: "off", Detail: err.Error()}
		default:
			out["habit"] = linkResult{State: "error", Detail: err.Error()}
		}
	}
	_ = s.store.SetWorkoutLinks(ctx, uid, wk.ID, healthRef, healthErr, habitDone)

	// the Core bus: once per workout, then each record, then the goal
	if s.once(ctx, fmt.Sprintf("done:%d:%d", uid, wk.ID)) {
		ex, _ := s.store.ExercisesByID(ctx, uid)
		names := []string{}
		seen := map[int64]bool{}
		for _, st := range wk.Sets {
			if e := ex[st.ExerciseID]; e != nil && !seen[e.ID] {
				seen[e.ID] = true
				names = append(names, e.Name)
			}
		}
		s.publish(ctx, uid, "fitness.workout_done", "workout:"+itoa(wk.ID), map[string]any{
			"workout_id": wk.ID, "name": wk.Name, "date": wk.Date, "started_at": wk.StartedAt, "duration_min": wk.DurationS / 60,
			"exercises": names, "sets": wk.SetCount, "volume_kg": wk.Volume, "records": wk.Records, "url": s.appLink("/workouts/" + itoa(wk.ID)),
		})
		em := map[string]*store.Exercise{}
		for k, v := range ex {
			em[itoa(k)] = v
		}
		for _, r := range workoutRecords(wk, em, st.WeightUnit) {
			s.publish(ctx, uid, "fitness.record", fmt.Sprintf("record:%d:%s:%s", r.ExerciseID, r.Kinds[0], wk.Date), map[string]any{
				"exercise": r.Exercise, "kinds": r.Kinds, "weight_kg": r.Weight, "reps": r.Reps, "duration_s": r.DurationS, "distance_m": r.DistanceM,
				"e1rm_kg": r.E1RM, "workout_id": wk.ID, "date": wk.Date, "text": r.Text,
			})
		}
		wv := s.week(ctx, uid, wk.Date)
		if wv.Goal > 0 && wv.Done >= wv.Goal && s.once(ctx, fmt.Sprintf("goal:%d:%s", uid, wv.Start)) {
			s.publish(ctx, uid, "fitness.goal_met", "week:"+wv.Start, map[string]any{"week": wv.Start, "done": wv.Done, "goal": wv.Goal,
				"text": fmt.Sprintf("Weekly goal met: %d of %d workouts.", wv.Done, wv.Goal)})
		}
	}
	return out
}

// recordText says a set's records in words, the main one first
// ("Bench Press (Barbell): 85 kg × 5, heaviest yet").
func recordText(ex string, st store.Set, kinds []string, unit string) string {
	if len(kinds) == 0 {
		return ex
	}
	what := map[string]string{"weight": "heaviest yet", "e1rm": "best estimated max yet", "volume": "most weight in one set yet",
		"reps": "most reps yet", "duration": "longest yet", "distance": "furthest yet"}[kinds[0]]
	if kinds[0] == "e1rm" {
		what += " (" + kgWords(st.E1RM(), unit) + ")"
	}
	return ex + ": " + setWords(st, unit) + ", " + what
}

func trimNum(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func clockDur(sec int) string {
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// appLink is a page of this app for messages and events.
func (s *Server) appLink(path string) string {
	if s.cfg.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(s.cfg.BaseURL, "/") + path
}

// healthKind is how Pocket Health calls a workout: strength, unless it is
// only running, cycling, rowing, walking or swimming.
func healthKind(wk *store.Workout, ex map[int64]*store.Exercise) string {
	kinds := map[string]bool{}
	for _, st := range wk.Sets {
		e := ex[st.ExerciseID]
		if e == nil || e.Kind != "cardio" {
			return "strength"
		}
		k := "other"
		switch {
		case strings.Contains(e.Builtin, "running") || e.Builtin == "treadmill":
			k = "running"
		case strings.Contains(e.Builtin, "cycling") || e.Builtin == "stationary-bike":
			k = "cycling"
		case e.Builtin == "rowing-machine":
			k = "rowing"
		case e.Builtin == "walking":
			k = "walking"
		case e.Builtin == "swimming":
			k = "swimming"
		case e.Builtin == "elliptical":
			k = "elliptical"
		}
		kinds[k] = true
	}
	if len(kinds) == 1 {
		for k := range kinds {
			if k != "other" {
				return k
			}
		}
	}
	return "functional"
}

// sendToHealth records a finished workout in Pocket Health. Health keeps one
// per start time, kind and app, so sending again updates it.
func (s *Server) sendToHealth(ctx context.Context, uid int64, wk *store.Workout) (string, error) {
	c := s.clients(ctx, uid)
	if !c.st.HealthSend {
		return "", &notConnected{"switched off"}
	}
	sib, err := c.conn(ctx, appHealth, scopeHealthWrite)
	if err != nil {
		return "", err
	}
	ex, _ := s.store.ExercisesByID(ctx, uid)
	kg := 75.0
	if wk.Bodyweight != nil && *wk.Bodyweight > 30 {
		kg = *wk.Bodyweight
	}
	// weight training ≈ 5 MET, cardio from the distance and time is Health's job
	kcal := math.Round(5 * kg * float64(wk.DurationS) / 3600)
	var dist float64
	names := []string{}
	seen := map[int64]bool{}
	for _, st := range wk.Sets {
		if st.DistanceM != nil && st.Done {
			dist += *st.DistanceM
		}
		if e := ex[st.ExerciseID]; e != nil && !seen[e.ID] {
			seen[e.ID] = true
			names = append(names, e.Name)
		}
	}
	notes := fmt.Sprintf("%d exercises, %d sets", wk.Exercises, wk.SetCount)
	if wk.Volume > 0 {
		notes += fmt.Sprintf(", %s kg lifted", trimNum(wk.Volume))
	}
	notes += ": " + strings.Join(names, ", ") + ". From Pocket Fitness."
	start, _ := time.Parse(time.RFC3339Nano, wk.StartedAt)
	end, _ := time.Parse(time.RFC3339Nano, wk.EndedAt)
	id, err := sib.client.SendWorkout(ctx, pocket.NewHealthWorkout{Kind: healthKind(wk, ex), Name: wk.Name, StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339),
		DurationS: wk.DurationS, DistanceM: dist, EnergyKcal: kcal, Notes: notes})
	if err != nil {
		return "", err
	}
	ref := itoa(id)
	if id == 0 {
		ref = "sent"
	}
	if ref != wk.HealthRef {
		_ = s.store.SetWorkoutLinks(ctx, uid, wk.ID, ref, "", wk.HabitDone)
	}
	return ref, nil
}

// tickHabit ticks the chosen Pocket Habits habit on a date.
func (s *Server) tickHabit(ctx context.Context, uid int64, st store.Settings, date string) error {
	sib, err := s.clients(ctx, uid).conn(ctx, appHabits, scopeEntriesWrite)
	if err != nil {
		return err
	}
	return sib.client.MarkHabit(ctx, st.HabitID, date)
}

// cached keeps a sibling app's answer for a while (per person and key).
func cached[T any](s *Server, ctx context.Context, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	if raw, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < ttl {
			var v T
			if json.Unmarshal([]byte(raw), &v) == nil {
				return v, nil
			}
		}
	}
	v, err := fetch()
	if err != nil {
		return v, err
	}
	if b, err := json.Marshal(v); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	return v, nil
}

// healthWorkouts are the person's other workouts from Pocket Health in a
// range (the ones Fitness sent, or starting within ten minutes of one of
// its own, left out), nil when Health is not linked or switched off here.
func (s *Server) healthWorkouts(ctx context.Context, uid int64, from, to string) []pocket.HealthWorkout {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.HealthCount {
		return nil
	}
	var list []pocket.HealthWorkout
	if s.demoApps && uid == s.store.OwnerID(ctx) {
		list = demoHealthWorkouts(from, to)
	} else {
		var err error
		list, err = cached(s, ctx, fmt.Sprintf("hw:%d:%s:%s", uid, from, to), 10*time.Minute, func() ([]pocket.HealthWorkout, error) {
			sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthRead)
			if err != nil {
				return nil, err
			}
			return sib.client.HealthWorkouts(ctx, from, to)
		})
		if err != nil {
			return nil
		}
	}
	own, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{From: store.AddDays(from, -1), To: store.AddDays(to, 1), Limit: 500})
	var starts []time.Time
	for _, w := range own {
		if t, err := time.Parse(time.RFC3339Nano, w.StartedAt); err == nil {
			starts = append(starts, t)
		}
	}
	out := []pocket.HealthWorkout{}
	for _, h := range list {
		if h.Source == "fitness" || h.Date < from || h.Date > to {
			continue
		}
		if t, err := time.Parse(time.RFC3339Nano, h.StartAt); err == nil {
			dup := false
			for _, o := range starts {
				if math.Abs(t.Sub(o).Minutes()) < 10 {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
		}
		out = append(out, h)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].StartAt < out[b].StartAt })
	return out
}

// weightSeries is the person's weight (kg) of the last days from Pocket
// Health, oldest first, nil when there is none.
func (s *Server) weightSeries(ctx context.Context, uid int64, days int) []pocket.WeightPoint {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.HealthWeight {
		return nil
	}
	if s.demoApps && uid == s.store.OwnerID(ctx) {
		return demoWeight(days)
	}
	pts, err := cached(s, ctx, fmt.Sprintf("weight:%d:%d", uid, days), time.Hour, func() ([]pocket.WeightPoint, error) {
		sib, err := s.clients(ctx, uid).conn(ctx, appHealth, scopeHealthRead)
		if err != nil {
			return nil, err
		}
		return sib.client.Weight(ctx, days)
	})
	if err != nil {
		return nil
	}
	return pts
}

// bodyweight is the person's latest weight: Pocket Health's, else their own
// setting, else 0.
func (s *Server) bodyweight(ctx context.Context, uid int64) float64 {
	if pts := s.weightSeries(ctx, uid, 90); len(pts) > 0 {
		return round1(pts[len(pts)-1].Value)
	}
	st, _ := s.store.GetSettings(ctx, uid)
	return st.BodyweightKg
}

// weatherFor is the weather of a date for an outdoor session, nil when
// Pocket Weather is not linked or the person switched it off.
func (s *Server) weatherFor(ctx context.Context, uid int64, date string) *pocket.WeatherDay {
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.OutdoorWeather {
		return nil
	}
	if s.demoApps && uid == s.store.OwnerID(ctx) {
		return &pocket.WeatherDay{Date: date, Place: "Sarajevo", Text: "Dry and mild, 9 to 16 °C, a light breeze in the afternoon — a good day to run outside."}
	}
	wd, err := cached(s, ctx, fmt.Sprintf("wx:%d:%s", uid, date), 30*time.Minute, func() (*pocket.WeatherDay, error) {
		sib, err := s.clients(ctx, uid).conn(ctx, appWeather, scopeWeatherRead)
		if err != nil {
			return nil, err
		}
		return sib.client.Day(ctx, date)
	})
	if err != nil {
		return nil
	}
	return wd
}

// ---- Pocket Calendar: the plan for the next two weeks ----

// routineMinutes guesses how long a routine takes (sets, work and rest),
// rounded to a quarter of an hour.
func routineMinutes(r *store.Routine, rest int) int {
	sec := 0
	for _, it := range r.Items {
		rs := rest
		if it.RestS != nil {
			rs = *it.RestS
		}
		per := 45
		if it.DurationS != nil && *it.DurationS > 0 {
			per = *it.DurationS
		}
		sec += it.Sets * (per + rs)
	}
	m := int(math.Ceil(float64(sec)/60/15)) * 15
	return max(30, min(m, 180))
}

// syncCalendar puts the planned sessions of the next 14 days on the
// person's calendar, moves the ones whose plan changed and removes the ones
// no longer planned (or all of them when switched off).
func (s *Server) syncCalendar(ctx context.Context, uid int64) (int, error) {
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil {
		return 0, err
	}
	have, err := s.store.PlanEvents(ctx, uid)
	if err != nil {
		return 0, err
	}
	if !st.CalendarPlan && len(have) == 0 {
		return 0, nil
	}
	sib, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return 0, err
	}
	today := store.Today()
	want := map[string]store.PlanEvent{}
	routines := map[int64]*store.Routine{}
	if st.CalendarPlan {
		for i := 0; i < 14; i++ {
			d := store.AddDays(today, i)
			p, _ := s.store.PlanFor(ctx, uid, d)
			if p == nil {
				continue
			}
			if i == 0 && s.doneOn(ctx, uid, d) && have[d].EventID == 0 {
				continue
			}
			want[d] = store.PlanEvent{Date: d, RoutineID: p.RoutineID, Time: p.Time}
			if routines[p.RoutineID] == nil {
				routines[p.RoutineID], _ = s.store.GetRoutine(ctx, uid, p.RoutineID)
			}
		}
	}
	changed := 0
	for d, h := range have {
		if d < today {
			continue
		}
		if _, ok := want[d]; !ok {
			if err := sib.client.DeleteEvent(ctx, h.EventID); err != nil {
				return changed, err
			}
			_ = s.store.SavePlanEvent(ctx, uid, store.PlanEvent{Date: d})
			changed++
		}
	}
	for d, w := range want {
		r := routines[w.RoutineID]
		if r == nil {
			continue
		}
		ev := pocket.Event{CalendarID: st.CalendarID, Title: r.Name, URL: s.appLink("/"), Start: d, Minutes: routineMinutes(r, st.RestS),
			Description: fmt.Sprintf("Planned in Pocket Fitness: %d %s.", len(r.Items), plural(len(r.Items), "exercise", "exercises"))}
		if w.Time != "" {
			ev.Start = d + "T" + w.Time
		}
		if h, ok := have[d]; ok {
			if h.RoutineID == w.RoutineID && h.Time == w.Time {
				continue
			}
			if err := sib.client.UpdateEvent(ctx, h.EventID, ev); err == nil {
				_ = s.store.SavePlanEvent(ctx, uid, store.PlanEvent{Date: d, RoutineID: w.RoutineID, Time: w.Time, EventID: h.EventID})
				changed++
				continue
			}
		}
		id, err := sib.client.CreateEvent(ctx, ev)
		if err != nil {
			return changed, err
		}
		_ = s.store.SavePlanEvent(ctx, uid, store.PlanEvent{Date: d, RoutineID: w.RoutineID, Time: w.Time, EventID: id})
		changed++
	}
	return changed, nil
}

// syncCalendarSoon syncs in the background after the plan or the setting changed.
func (s *Server) syncCalendarSoon(uid int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if _, err := s.syncCalendar(ctx, uid); err != nil && !isNotConnected(err) {
			s.log.Warn("calendar: sync failed", "user", uid, "err", err)
		}
	}()
}

// doneOn reports whether the person finished a workout on a date.
func (s *Server) doneOn(ctx context.Context, uid int64, date string) bool {
	days, _ := s.store.WorkoutDays(ctx, uid, date, date)
	return days[date] != nil
}

// hourly: workouts left running, and once a day each person's calendar.
func (s *Server) hourly(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		s.finishStale(ctx, uid)
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.CalendarPlan {
			continue
		}
		if s.once(ctx, fmt.Sprintf("calsync:%d:%s", uid, store.Today())) {
			if _, err := s.syncCalendar(ctx, uid); err != nil && !isNotConnected(err) {
				s.log.Warn("calendar: sync failed", "user", uid, "err", err)
			}
		}
	}
}

// ---- lists for Settings: habits, calendars, playlists ----

func (s *Server) handleAppHabits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.demoApps && actorID(ctx) == s.store.OwnerID(ctx) {
		writeJSON(w, http.StatusOK, []pocket.Habit{{ID: 1, Name: "Gym"}, {ID: 2, Name: "Move every day"}, {ID: 3, Name: "Read"}})
		return
	}
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appHabits, scopeHabitsRead)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	list, err := sib.client.Habits(ctx)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAppCalendars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appCalendar, scopeCalendarRead)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	list, err := sib.client.Calendars(ctx)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAppPlaylists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.demoApps && actorID(ctx) == s.store.OwnerID(ctx) {
		writeJSON(w, http.StatusOK, []pocket.Playlist{{ID: "pl1", Name: "Gym bangers", SongCount: 48}, {ID: "pl2", Name: "Long run", SongCount: 31}})
		return
	}
	sib, err := s.clients(ctx, actorID(ctx)).conn(ctx, appMusic, scopePlaylists)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	list, err := sib.client.Playlists(ctx)
	if err != nil {
		s.siblingErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCalendarSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := s.syncCalendar(ctx, actorID(ctx))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": n})
}

// siblingErr: not connected is a 409 with the reason in plain words.
func (s *Server) siblingErr(w http.ResponseWriter, err error) {
	if isNotConnected(err) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeErr(w, http.StatusBadGateway, err.Error())
}

// playlistURL is where the chosen playlist opens in Pocket Music.
func (s *Server) playlistURL(ctx context.Context, uid int64, st store.Settings) string {
	if st.PlaylistID == "" {
		return ""
	}
	return s.siblingLink(ctx, uid, appMusic, "/playlists/"+st.PlaylistID)
}

// ---- made-up linked apps for the demo (DEMO_DATA without Core) ----

func demoHealthWorkouts(from, to string) []pocket.HealthWorkout {
	var out []pocket.HealthWorkout
	for d := from; d <= to; d = store.AddDays(d, 1) {
		t, _ := store.ParseDate(d)
		if d > store.Today() {
			break
		}
		rng := rand.New(rand.NewSource(t.Unix()))
		switch t.Weekday() {
		case time.Tuesday, time.Saturday:
			km := 5 + rng.Float64()*4
			if t.Weekday() == time.Saturday {
				km += 3
			}
			dur := int(km * (330 + rng.Float64()*40))
			start := time.Date(t.Year(), t.Month(), t.Day(), 7, 10, 0, 0, time.Local)
			out = append(out, pocket.HealthWorkout{ID: t.Unix(), Kind: "running", Name: "Morning run", Date: d, StartAt: start.Format(time.RFC3339),
				EndAt: start.Add(time.Duration(dur) * time.Second).Format(time.RFC3339), DurationS: dur, DistanceM: math.Round(km * 1000), EnergyKcal: math.Round(km * 68), AvgHR: 148 + rng.Float64()*10, Source: "apple"})
		case time.Sunday:
			if rng.Intn(2) == 0 {
				start := time.Date(t.Year(), t.Month(), t.Day(), 10, 30, 0, 0, time.Local)
				out = append(out, pocket.HealthWorkout{ID: t.Unix(), Kind: "cycling", Name: "Ride to Vrelo Bosne", Date: d, StartAt: start.Format(time.RFC3339),
					EndAt: start.Add(95 * time.Minute).Format(time.RFC3339), DurationS: 95 * 60, DistanceM: 31200, EnergyKcal: 640, Source: "apple"})
			}
		}
	}
	return out
}

func demoWeight(days int) []pocket.WeightPoint {
	var out []pocket.WeightPoint
	today := store.Today()
	rng := rand.New(rand.NewSource(7))
	for i := days - 1; i >= 0; i-- {
		if i%3 != 0 {
			continue
		}
		v := 82.7 - float64(i)*0.012 + (rng.Float64()-0.5)*0.8
		out = append(out, pocket.WeightPoint{Date: store.AddDays(today, -i), Value: round1(v)})
	}
	return out
}

// testDetail adds what a connection test found ("4 habits").
func (s *Server) testDetail(ctx context.Context, a pocketApp, conn *siblingConn) string {
	switch a.slug {
	case "habits":
		if l, err := conn.client.Habits(ctx); err == nil {
			return fmt.Sprintf("; %d %s", len(l), plural(len(l), "habit", "habits"))
		}
	case "calendar":
		if l, err := conn.client.Calendars(ctx); err == nil {
			return fmt.Sprintf("; %d %s you can add to", len(l), plural(len(l), "calendar", "calendars"))
		}
	case "music":
		if l, err := conn.client.Playlists(ctx); err == nil {
			return fmt.Sprintf("; %d %s", len(l), plural(len(l), "playlist", "playlists"))
		}
	case "health":
		if l, err := conn.client.Weight(ctx, 90); err == nil && len(l) > 0 {
			return fmt.Sprintf("; your weight: %s kg", trimNum(l[len(l)-1].Value))
		}
	}
	return ""
}
