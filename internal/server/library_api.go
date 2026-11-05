package server

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// ---- exercises ----

type exerciseInput struct {
	Name      *string `json:"name"`
	Muscle    *string `json:"muscle"`
	Secondary *string `json:"secondary"`
	Equipment *string `json:"equipment"`
	Kind      *string `json:"kind"`
	Notes     *string `json:"notes"`
	Archived  *bool   `json:"archived"`
}

func (in exerciseInput) apply(e *store.Exercise) string {
	trimP(&e.Name, in.Name)
	if e.Name == "" || len(e.Name) > 100 {
		return "an exercise needs a name of up to 100 characters"
	}
	if in.Muscle != nil {
		if !store.ValidMuscle(*in.Muscle) {
			return "unknown muscle"
		}
		e.Muscle = *in.Muscle
	}
	if in.Secondary != nil {
		var keep []string
		for _, m := range strings.Split(*in.Secondary, ",") {
			if m = strings.TrimSpace(m); store.ValidMuscle(m) && m != e.Muscle {
				keep = append(keep, m)
			}
		}
		e.Secondary = strings.Join(keep, ",")
	}
	if in.Equipment != nil {
		if !store.ValidEquipment(*in.Equipment) {
			return "unknown equipment"
		}
		e.Equipment = *in.Equipment
	}
	if in.Kind != nil {
		if !store.ValidKind(*in.Kind) {
			return "an exercise is weight, reps, time or cardio"
		}
		e.Kind = *in.Kind
	}
	trimP(&e.Notes, in.Notes)
	if in.Archived != nil {
		e.Archived = *in.Archived
	}
	return ""
}

func (s *Server) handleListExercises(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListExercises(ctx, actorID(ctx), queryBool(r, "archived"))
	if err != nil {
		s.fail(w, err, "exercises")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateExercise(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in exerciseInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	e := &store.Exercise{UserID: actorID(ctx), Muscle: "other", Equipment: "other", Kind: "weight"}
	if msg := in.apply(e); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateExercise(ctx, e); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "you already have an exercise called "+e.Name)
			return
		}
		s.fail(w, err, "add exercise")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// handleGetExercise is one exercise with its records and history (and the
// points for its chart, oldest first).
func (s *Server) handleGetExercise(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	e, err := s.store.GetExercise(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "exercise")
		return
	}
	recs, _ := s.store.Records(ctx, uid, e.ID)
	hist, _ := s.store.ExerciseHistory(ctx, uid, e.ID, 200)
	type point struct {
		Date   string  `json:"date"`
		Best   float64 `json:"best"`
		E1RM   float64 `json:"e1rm"`
		Volume float64 `json:"volume"`
	}
	chart := []point{}
	for i := len(hist) - 1; i >= 0; i-- {
		chart = append(chart, point{hist[i].Date, round1(hist[i].Best), hist[i].E1RM, round1(hist[i].Volume)})
	}
	if len(hist) > 30 {
		hist = hist[:30]
	}
	writeJSON(w, http.StatusOK, map[string]any{"exercise": e, "records": recs, "history": hist, "chart": chart})
}

func (s *Server) handleExerciseHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	hist, err := s.store.ExerciseHistory(ctx, actorID(ctx), idParam(r, "id"), limit)
	if err != nil {
		s.fail(w, err, "history")
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func (s *Server) handleUpdateExercise(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e, err := s.store.GetExercise(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "exercise")
		return
	}
	var in exerciseInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(e); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateExercise(ctx, e); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "you already have an exercise called "+e.Name)
			return
		}
		s.fail(w, err, "save exercise")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleDeleteExercise(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hidden, err := s.store.DeleteExercise(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "delete exercise")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"archived": hidden})
}

// ---- routines ----

type routineItemIn struct {
	ExerciseID int64    `json:"exercise_id"`
	Exercise   string   `json:"exercise"` // a name (Pocket AI's routines)
	Sets       int      `json:"sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	Weight     *float64 `json:"weight"`
	DurationS  *int     `json:"duration_s"`
	DistanceM  *float64 `json:"distance_m"`
	RestS      *int     `json:"rest_s"`
	Notes      string   `json:"notes"`
}

type routineInput struct {
	Name     *string          `json:"name"`
	Notes    *string          `json:"notes"`
	Outdoors *bool            `json:"outdoors"`
	Items    *[]routineItemIn `json:"items"`
	Source   string           `json:"source"`
}

// applyRoutine checks the input against the person's exercises.
func (s *Server) applyRoutine(r *http.Request, uid int64, rt *store.Routine, in routineInput) (int, string) {
	trimP(&rt.Name, in.Name)
	if rt.Name == "" || len(rt.Name) > 100 {
		return http.StatusBadRequest, "a routine needs a name of up to 100 characters"
	}
	trimP(&rt.Notes, in.Notes)
	if in.Outdoors != nil {
		rt.Outdoors = *in.Outdoors
	}
	if in.Items == nil {
		return 0, ""
	}
	if len(*in.Items) > 60 {
		return http.StatusBadRequest, "a routine has at most 60 exercises"
	}
	ex, err := s.store.ExercisesByID(r.Context(), uid)
	if err != nil {
		return http.StatusInternalServerError, err.Error()
	}
	rt.Items = []store.RoutineItem{}
	for _, it := range *in.Items {
		e := ex[it.ExerciseID]
		if e == nil && it.Exercise != "" {
			if e, err = s.store.ResolveExercise(r.Context(), uid, it.Exercise, ""); err != nil {
				return http.StatusBadRequest, err.Error()
			}
		}
		if e == nil {
			return http.StatusNotFound, "not found"
		}
		if it.Sets <= 0 {
			it.Sets = 3
		}
		if it.Sets > 20 {
			return http.StatusBadRequest, "at most 20 sets of one exercise"
		}
		bad := func(p *int, lo, hi int) bool { return p != nil && (*p < lo || *p > hi) }
		if bad(it.RepsMin, 0, 1000) || bad(it.RepsMax, 0, 1000) || bad(it.DurationS, 0, 86400) || bad(it.RestS, 0, 3600) ||
			(it.Weight != nil && (*it.Weight < 0 || *it.Weight > 2000)) || (it.DistanceM != nil && (*it.DistanceM < 0 || *it.DistanceM > 1e6)) {
			return http.StatusBadRequest, "a number in the routine is out of range"
		}
		if it.RepsMin != nil && it.RepsMax != nil && *it.RepsMin > *it.RepsMax {
			it.RepsMin, it.RepsMax = it.RepsMax, it.RepsMin
		}
		rt.Items = append(rt.Items, store.RoutineItem{ExerciseID: e.ID, Sets: it.Sets, RepsMin: it.RepsMin, RepsMax: it.RepsMax, Weight: it.Weight,
			DurationS: it.DurationS, DistanceM: it.DistanceM, RestS: it.RestS, Notes: strings.TrimSpace(it.Notes)})
	}
	return 0, ""
}

func (s *Server) handleListRoutines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.store.ListRoutines(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "routines")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rt, err := s.store.GetRoutine(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "routine")
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

func (s *Server) handleCreateRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in routineInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rt := &store.Routine{UserID: uid, Items: []store.RoutineItem{}}
	if oneOf(in.Source, []string{"ai", "import"}) {
		rt.Source = in.Source
	}
	if code, msg := s.applyRoutine(r, uid, rt, in); code != 0 {
		writeErr(w, code, msg)
		return
	}
	if err := s.store.SaveRoutine(ctx, rt); err != nil {
		s.fail(w, err, "save routine")
		return
	}
	got, _ := s.store.GetRoutine(ctx, uid, rt.ID)
	writeJSON(w, http.StatusCreated, got)
}

func (s *Server) handleUpdateRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	rt, err := s.store.GetRoutine(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "routine")
		return
	}
	var in routineInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if code, msg := s.applyRoutine(r, uid, rt, in); code != 0 {
		writeErr(w, code, msg)
		return
	}
	if err := s.store.SaveRoutine(ctx, rt); err != nil {
		s.fail(w, err, "save routine")
		return
	}
	got, _ := s.store.GetRoutine(ctx, uid, rt.ID)
	s.syncCalendarSoon(uid) // a new name or length on the calendar
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) handleDeleteRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if err := s.store.DeleteRoutine(ctx, uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete routine")
		return
	}
	s.syncCalendarSoon(uid)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOrderRoutines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.OrderRoutines(ctx, actorID(ctx), in.IDs); err != nil {
		s.fail(w, err, "order")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- the week's plan ----

func (s *Server) handleGetPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	plan, err := s.store.GetPlan(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "plan")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// handlePutPlan replaces the week: [{weekday, routine_id, time}], a day
// left out is a rest day.
func (s *Server) handlePutPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in []store.PlanDay
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	seen := map[int]bool{}
	var days []store.PlanDay
	for _, d := range in {
		if d.Weekday < 1 || d.Weekday > 7 || seen[d.Weekday] {
			writeErr(w, http.StatusBadRequest, "a weekday is 1 (Monday) to 7 (Sunday), once each")
			return
		}
		seen[d.Weekday] = true
		if d.RoutineID == 0 {
			continue
		}
		if _, err := s.store.GetRoutine(ctx, uid, d.RoutineID); err != nil {
			s.fail(w, err, "routine")
			return
		}
		if d.Time != "" && !hhmm.MatchString(d.Time) {
			writeErr(w, http.StatusBadRequest, "a time is HH:MM")
			return
		}
		days = append(days, store.PlanDay{Weekday: d.Weekday, RoutineID: d.RoutineID, Time: d.Time})
	}
	if err := s.store.SetPlan(ctx, uid, days); err != nil {
		s.fail(w, err, "save plan")
		return
	}
	s.syncCalendarSoon(uid)
	plan, _ := s.store.GetPlan(ctx, uid)
	writeJSON(w, http.StatusOK, plan)
}
