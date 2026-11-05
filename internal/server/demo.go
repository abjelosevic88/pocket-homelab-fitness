package server

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// seedDemo gives the owner six months of training (DEMO_DATA): four
// routines on a weekly plan, kept most weeks, with the weights going up
// the way they do (a little at a time, with the odd bad day).
func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	// with AUTH_MODE=none this runs inside the first request; finish even
	// when that client goes away
	ctx = context.WithoutCancel(ctx)
	if l, _ := s.store.ListRoutines(ctx, uid); len(l) > 0 {
		return nil
	}
	if err := s.store.EnsureCatalog(ctx, uid); err != nil {
		return err
	}
	rng := rand.New(rand.NewSource(42))
	ex := func(name string) *store.Exercise {
		e, err := s.store.ResolveExercise(ctx, uid, name, "")
		if err != nil {
			panic(err)
		}
		return e
	}
	ip := func(v int) *int { return &v }
	type plan struct {
		ex         *store.Exercise
		sets       int
		reps       int
		start, inc float64 // kg, and the step up
		warmup     bool
	}
	routines := []struct {
		name, notes string
		outdoors    bool
		weekday     int
		time        string
		items       []plan
	}{
		{"Push day", "Chest, shoulders and triceps.", false, 1, "18:00", []plan{
			{ex("Bench Press (Barbell)"), 3, 8, 62.5, 2.5, true}, {ex("Overhead Press (Barbell)"), 3, 8, 37.5, 2.5, false},
			{ex("Incline Bench Press (Dumbbell)"), 3, 10, 20, 2, false}, {ex("Lateral Raise (Dumbbell)"), 3, 12, 8, 1, false},
			{ex("Triceps Pushdown (Cable)"), 3, 12, 25, 2.5, false}}},
		{"Pull day", "Back and biceps.", false, 3, "18:00", []plan{
			{ex("Deadlift (Barbell)"), 3, 5, 95, 5, true}, {ex("Pull Up"), 3, 8, 0, 0, false},
			{ex("Bent Over Row (Barbell)"), 3, 8, 50, 2.5, false}, {ex("Face Pull (Cable)"), 3, 15, 20, 2.5, false},
			{ex("Bicep Curl (Dumbbell)"), 3, 10, 12, 1, false}}},
		{"Park circuit", "A short run and bodyweight work in the park.", true, 4, "07:30", []plan{
			{ex("Running"), 1, 0, 0, 0, false}, {ex("Push Up"), 3, 15, 0, 0, false}, {ex("Chin Up"), 3, 6, 0, 0, false},
			{ex("Lunge (Dumbbell)"), 3, 12, 0, 0, false}, {ex("Plank"), 3, 0, 0, 0, false}}},
		{"Leg day", "Squats first, while fresh.", false, 5, "17:30", []plan{
			{ex("Squat (Barbell)"), 3, 6, 80, 2.5, true}, {ex("Romanian Deadlift (Barbell)"), 3, 8, 60, 2.5, false},
			{ex("Leg Press (Machine)"), 3, 10, 120, 5, false}, {ex("Seated Leg Curl (Machine)"), 3, 12, 35, 2.5, false},
			{ex("Standing Calf Raise (Machine)"), 3, 15, 50, 5, false}}},
	}
	var days []store.PlanDay
	type state struct {
		weight float64
		reps   int
		good   int
	}
	cur := map[int64]*state{}
	ids := make([]int64, len(routines))
	for i, r := range routines {
		rt := &store.Routine{UserID: uid, Name: r.name, Notes: r.notes, Outdoors: r.outdoors}
		for _, it := range r.items {
			item := store.RoutineItem{ExerciseID: it.ex.ID, Sets: it.sets}
			switch it.ex.Kind {
			case "time":
				item.DurationS = ip(60)
			case "cardio":
				d := 2000.0
				item.DistanceM = &d
			default:
				item.RepsMin, item.RepsMax = ip(it.reps-2), ip(it.reps)
				if it.reps <= 6 {
					item.RepsMin = ip(it.reps - 1)
				}
				if it.ex.Muscle == "quads" || it.ex.Name == "Deadlift (Barbell)" {
					item.RestS = ip(180)
				}
			}
			rt.Items = append(rt.Items, item)
			cur[it.ex.ID] = &state{weight: it.start, reps: it.reps}
		}
		if err := s.store.SaveRoutine(ctx, rt); err != nil {
			return err
		}
		ids[i] = rt.ID
		days = append(days, store.PlanDay{Weekday: r.weekday, RoutineID: rt.ID, Time: r.time})
	}
	if err := s.store.SetPlan(ctx, uid, days); err != nil {
		return err
	}
	st, _ := s.store.GetSettings(ctx, uid)
	st.WeekGoal, st.BodyweightKg = 5, 82
	st.HabitID, st.HabitName = 1, "Gym"
	st.PlaylistID, st.PlaylistName = "pl1", "Gym bangers"
	_ = s.store.SaveUserSettings(ctx, uid, st)

	notes := []string{"Felt strong today.", "Short on sleep, kept it light.", "New shoes.", "Gym was packed, swapped the order.", "Shoulder a bit stiff, easy on the press."}
	today := store.Today()
	start := store.AddDays(today, -182)
	for d := start; d < today; d = store.AddDays(d, 1) {
		t, _ := store.ParseDate(d)
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		ri := -1
		for i, r := range routines {
			if r.weekday == wd {
				ri = i
			}
		}
		if ri < 0 || rng.Float64() < 0.12 { // a rest day, or a session missed
			continue
		}
		r := routines[ri]
		ago := int(time.Since(t).Hours() / 24)
		tt, _ := time.Parse("15:04", r.time)
		hh, mm := tt.Hour(), tt.Minute()
		begin := time.Date(t.Year(), t.Month(), t.Day(), hh, mm, 0, 0, time.Local).Add(time.Duration(rng.Intn(25)-5) * time.Minute)
		rid := ids[ri]
		wk := &store.Workout{UserID: uid, Name: r.name, RoutineID: &rid, StartedAt: rfc(begin), Date: d, Feel: 3 + rng.Intn(3)}
		bw := round1(82.7 - float64(ago)*0.012 + (rng.Float64()-0.5)*0.6)
		wk.Bodyweight = &bw
		if rng.Float64() < 0.15 {
			wk.Notes = notes[rng.Intn(len(notes))]
		}
		bad := rng.Float64() < 0.15
		for pos, it := range r.items {
			c := cur[it.ex.ID]
			add := func(st store.Set) {
				st.ExerciseID, st.Position, st.Done = it.ex.ID, pos, true
				if st.Type == "" {
					st.Type = "normal"
				}
				wk.Sets = append(wk.Sets, st)
			}
			if it.warmup {
				w1, w2 := math.Round(c.weight*0.5/2.5)*2.5, math.Round(c.weight*0.75/2.5)*2.5
				add(store.Set{Type: "warmup", Weight: &w1, Reps: ip(8)})
				add(store.Set{Type: "warmup", Weight: &w2, Reps: ip(4)})
			}
			allHit := true
			for i := 0; i < it.sets; i++ {
				switch it.ex.Kind {
				case "cardio":
					dist := math.Round(1800 + rng.Float64()*700)
					add(store.Set{DistanceM: &dist, DurationS: ip(int(dist * (0.33 + rng.Float64()*0.04)))})
				case "time":
					add(store.Set{DurationS: ip(45 + 15*rng.Intn(3) + (182-ago)/30*5)})
				case "reps":
					reps := it.reps + (182-ago)/45 - rng.Intn(3)
					if bad {
						reps -= 2
					}
					add(store.Set{Reps: ip(max(reps, 3))})
				default:
					reps := c.reps
					if i == it.sets-1 && rng.Float64() < 0.45 || bad {
						reps -= 1 + rng.Intn(2)
						allHit = false
					}
					w := c.weight
					rpe := 7.5 + float64(i)*0.5
					add(store.Set{Weight: &w, Reps: ip(reps), RPE: &rpe})
				}
			}
			if it.ex.Kind == "weight" {
				if allHit {
					c.good++
				}
				if c.good >= 2 {
					c.weight += it.inc
					c.good = 0
				}
			}
		}
		mins := 12 + len(wk.Sets)*3 + rng.Intn(10)
		wk.EndedAt, wk.DurationS = rfc(begin.Add(time.Duration(mins)*time.Minute)), mins*60
		wk.HealthRef, wk.HabitDone = "demo", d
		if err := s.store.SaveWorkout(ctx, wk); err != nil {
			return err
		}
		_ = s.store.SetWorkoutLinks(ctx, uid, wk.ID, "demo", "", d)
	}
	return nil
}
