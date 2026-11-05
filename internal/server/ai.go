package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// aiFor is the person's AI: Pocket AI through Core, their own
// OpenAI-compatible key, or nil with why.
func (s *Server) aiFor(ctx context.Context, uid int64, st store.Settings) (*ai.Client, string, string) {
	email := ""
	if u, err := s.store.GetUser(ctx, uid); err == nil {
		email = u.Email
	}
	pocket, status := s.core.SelectAI(ctx, email, st.AIProvider, st.OpenAIKey != "")
	if pocket != nil {
		if !status.Ready {
			return nil, "", "Pocket AI: " + status.Detail
		}
		c := ai.New("", "", "", 150*time.Second)
		c.Pocket = pocket.WithModel(st.AIPocketModel)
		return c, "pocket", ""
	}
	if st.OpenAIKey == "" {
		return nil, "", status.Detail
	}
	return ai.New(st.OpenAIBaseURL, st.OpenAIKey, st.OpenAIModel, 150*time.Second), "openai", ""
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, status := s.core.SelectAI(ctx, userFrom(ctx).Email, "pocket", false)
	if c == nil || !status.Ready {
		writeErr(w, http.StatusServiceUnavailable, status.Detail)
		return
	}
	models, err := c.Models(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func validAIProvider(v string) bool { return core.ValidAIProvider(v) }

func validPocketModel(v string) bool { return core.ValidInferenceModel(v) }

const aiVoice = `You are the training log app of one person and talk like a good gym friend who looked at their log: plainly and briefly, no jargon, no emojis, no markdown headings, no greeting. Only use the facts you are given; never invent numbers. Weights are in the unit you are told.`

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	cl, used, why := s.aiFor(ctx, actorID(ctx), st)
	writeJSON(w, http.StatusOK, map[string]any{"ready": cl != nil, "provider": used, "detail": why, "review": st.AIReview})
}

// trainingFacts is a person's training of a period as text for the AI: the
// workouts with their exercises, the week goal, records, muscles, other
// workouts from Pocket Health and the weight trend.
func (s *Server) trainingFacts(ctx context.Context, uid int64, st store.Settings, from, to string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today is %s. Weights in %s. Weekly goal: %d workouts.\n", time.Now().Format("Monday 2 January 2006"), st.WeightUnit, st.WeekGoal)
	list, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{From: from, To: to, Finished: true, Limit: 200})
	ex, _ := s.store.ExercisesByID(ctx, uid)
	fmt.Fprintf(&b, "Workouts from %s to %s (%d):\n", from, to, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		w := list[i]
		t, _ := store.ParseDate(w.Date)
		fmt.Fprintf(&b, "- %s %s, %d min, %d sets", t.Format("Mon 2 Jan"), w.Name, w.DurationS/60, w.SetCount)
		if w.Feel > 0 {
			fmt.Fprintf(&b, ", felt %d/5", w.Feel)
		}
		b.WriteString(": ")
		pos := -1
		var parts []string
		var cur []string
		name := ""
		flush := func() {
			if name != "" {
				parts = append(parts, name+" "+strings.Join(cur, ", "))
			}
		}
		for _, sx := range w.Sets {
			if sx.Position != pos {
				flush()
				pos, cur, name = sx.Position, nil, "?"
				if e := ex[sx.ExerciseID]; e != nil {
					name = e.Name
				}
			}
			if sx.Type == "warmup" {
				continue
			}
			word := setWords(sx, st.WeightUnit)
			if len(sx.PR) > 0 {
				word += " (record)"
			}
			cur = append(cur, word)
		}
		flush()
		b.WriteString(strings.Join(parts, "; ") + "\n")
	}
	if hw := s.healthWorkouts(ctx, uid, from, to); len(hw) > 0 {
		b.WriteString("Other workouts (from the watch):\n")
		for _, h := range hw {
			fmt.Fprintf(&b, "- %s %s, %d min", h.Date, healthItem(h).Name, h.DurationS/60)
			if h.DistanceM > 0 {
				fmt.Fprintf(&b, ", %s km", trimNum(h.DistanceM/1000))
			}
			b.WriteString("\n")
		}
	}
	if ms, _ := s.store.MuscleSets(ctx, uid, from, to); len(ms) > 0 {
		var parts []string
		for _, m := range store.Muscles {
			if v := ms[m]; v > 0 {
				parts = append(parts, fmt.Sprintf("%s %s", strings.ReplaceAll(m, "_", " "), trimNum(v)))
			}
		}
		b.WriteString("Sets per muscle: " + strings.Join(parts, ", ") + "\n")
	}
	wk := s.week(ctx, uid, store.Today())
	fmt.Fprintf(&b, "This week so far: %d of %d. Weeks in a row on goal: %d.\n", wk.Done, wk.Goal, s.streak(ctx, uid))
	if pts := s.weightSeries(ctx, uid, 60); len(pts) > 1 {
		fmt.Fprintf(&b, "Body weight: %s kg on %s, %s kg on %s.\n", trimNum(pts[0].Value), pts[0].Date, trimNum(pts[len(pts)-1].Value), pts[len(pts)-1].Date)
	}
	if plan, _ := s.store.GetPlan(ctx, uid); len(plan) > 0 {
		var parts []string
		for _, p := range plan {
			parts = append(parts, time.Weekday(p.Weekday%7).String()+" "+p.Routine)
		}
		b.WriteString("Their plan: " + strings.Join(parts, ", ") + ".\n")
	}
	return b.String()
}

// handleAIReview writes how the week (or the month) went in plain words,
// with one thing to keep and one to try. Kept for the day.
func (s *Server) handleAIReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Period string `json:"period"` // week | month
		Fresh  bool   `json:"fresh"`
	}
	_ = decode(r, &in)
	if in.Period != "month" {
		in.Period = "week"
	}
	st, _ := s.store.GetSettings(ctx, uid)
	today := store.Today()
	from := store.AddDays(today, -13)
	if in.Period == "month" {
		from = store.AddDays(today, -29)
	}
	key := fmt.Sprintf("ai:review:%d:%s:%s", uid, in.Period, today)
	if !in.Fresh {
		if raw, at, ok := s.store.CacheGet(ctx, key); ok {
			var out map[string]any
			if json.Unmarshal([]byte(raw), &out) == nil {
				out["cached"], out["at"] = true, at
				writeJSON(w, http.StatusOK, out)
				return
			}
		}
	}
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	what := "this week (compared with the week before)"
	if in.Period == "month" {
		what = "the last four weeks"
	}
	prompt := s.trainingFacts(ctx, uid, st, from, today) + fmt.Sprintf(`
Write about %s in 3 or 4 short sentences: how much they trained against their goal, what went up (records, heavier sets), anything uneven (a muscle group left out, long gaps) and one concrete thing to try next. If there were no workouts, say so kindly and suggest an easy start. Plain text only.`, what)
	text, err := cl.Complete(ctx, aiVoice, prompt, false)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	out := map[string]any{"ok": true, "provider": used, "text": strings.TrimSpace(text), "period": in.Period}
	if b, err := json.Marshal(out); err == nil {
		_ = s.store.CachePut(ctx, key, string(b))
	}
	out["at"] = store.Now()
	writeJSON(w, http.StatusOK, out)
}

// exerciseNames is the person's exercise list for the AI, by muscle.
func (s *Server) exerciseNames(ctx context.Context, uid int64) (string, map[string]*store.Exercise) {
	list, _ := s.store.ListExercises(ctx, uid, false)
	by := map[string][]string{}
	idx := map[string]*store.Exercise{}
	for i := range list {
		e := &list[i]
		by[e.Muscle] = append(by[e.Muscle], fmt.Sprintf("%s [%s]", e.Name, e.Kind))
		idx[strings.ToLower(e.Name)] = e
	}
	var b strings.Builder
	for _, m := range store.Muscles {
		if len(by[m]) > 0 {
			fmt.Fprintf(&b, "%s: %s\n", m, strings.Join(by[m], "; "))
		}
	}
	return b.String(), idx
}

// matchExercise finds the person's exercise the AI meant: exact, then the
// built-in list and aliases; nil when it is none of theirs.
func (s *Server) matchExercise(ctx context.Context, uid int64, idx map[string]*store.Exercise, name string) *store.Exercise {
	if e := idx[strings.ToLower(strings.TrimSpace(name))]; e != nil {
		return e
	}
	if c, ok := store.CatalogMatch(name); ok {
		if e := idx[strings.ToLower(c.Name)]; e != nil {
			return e
		}
	}
	n := store.NormName(name)
	for k, e := range idx {
		if store.NormName(k) == n {
			return e
		}
	}
	return nil
}

// handleAIRoutine drafts routines for a goal ("stronger, 3 days, dumbbells
// only, 45 minutes") from the person's own exercises. Nothing is saved:
// the page shows the drafts and saves the ones the person keeps.
func (s *Server) handleAIRoutine(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Goal      string   `json:"goal"`
		Days      int      `json:"days"`
		Minutes   int      `json:"minutes"`
		Equipment []string `json:"equipment"`
		Level     string   `json:"level"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Goal = strings.TrimSpace(in.Goal)
	if len(in.Goal) > 1000 {
		writeErr(w, http.StatusBadRequest, "say the goal in up to 1000 characters")
		return
	}
	if in.Days < 1 || in.Days > 7 {
		in.Days = 3
	}
	if in.Minutes < 15 || in.Minutes > 180 {
		in.Minutes = 60
	}
	if !oneOf(in.Level, []string{"new", "some", "experienced"}) {
		in.Level = "some"
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	names, idx := s.exerciseNames(ctx, uid)
	equip := "any"
	var eq []string
	for _, e := range in.Equipment {
		if store.ValidEquipment(e) {
			eq = append(eq, e)
		}
	}
	if len(eq) > 0 {
		equip = strings.Join(eq, ", ")
	}
	goal := in.Goal
	if goal == "" {
		goal = "get fitter and stronger, all round"
	}
	prompt := fmt.Sprintf(`Their goal: """%s"""
Days a week: %d. Minutes per session: %d. Equipment they have: %s. Experience: %s.
Recent training, to build on:
%s
Their exercise list (name [kind]) — use ONLY these exact names:
%s
Design %d routine(s), one per training day of the week, that fit the time (count about 2–3 minutes per set with rest).
Reply with JSON only: {"routines":[{"name":"short name like Push day","notes":"one sentence on what it is for","weekdays":[1],"items":[{"exercise":"exact name from the list","sets":3,"reps_min":8,"reps_max":12,"rest_s":90,"duration_s":null,"notes":""}]}]}
weekdays: 1 = Monday … 7 = Sunday, spread with rest days between. For time exercises use duration_s and leave reps empty; for cardio leave reps empty.`,
		goal, in.Days, in.Minutes, equip, in.Level, s.trainingFacts(ctx, uid, st, store.AddDays(store.Today(), -27), store.Today()), names, in.Days)
	var out struct {
		Routines []struct {
			Name     string `json:"name"`
			Notes    string `json:"notes"`
			Weekdays []int  `json:"weekdays"`
			Items    []struct {
				Exercise  string `json:"exercise"`
				Sets      int    `json:"sets"`
				RepsMin   *int   `json:"reps_min"`
				RepsMax   *int   `json:"reps_max"`
				RestS     *int   `json:"rest_s"`
				DurationS *int   `json:"duration_s"`
				Notes     string `json:"notes"`
			} `json:"items"`
		} `json:"routines"`
	}
	if err := cl.JSON(ctx, aiVoice, prompt, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type draftItem struct {
		ExerciseID int64           `json:"exercise_id"`
		Exercise   *store.Exercise `json:"exercise"`
		Sets       int             `json:"sets"`
		RepsMin    *int            `json:"reps_min"`
		RepsMax    *int            `json:"reps_max"`
		RestS      *int            `json:"rest_s"`
		DurationS  *int            `json:"duration_s"`
		Notes      string          `json:"notes"`
	}
	type draft struct {
		Name     string      `json:"name"`
		Notes    string      `json:"notes"`
		Weekdays []int       `json:"weekdays"`
		Items    []draftItem `json:"items"`
	}
	drafts := []draft{}
	skipped := []string{}
	for _, rt := range out.Routines {
		d := draft{Name: strings.TrimSpace(rt.Name), Notes: strings.TrimSpace(rt.Notes), Items: []draftItem{}, Weekdays: []int{}}
		for _, wd := range rt.Weekdays {
			if wd >= 1 && wd <= 7 {
				d.Weekdays = append(d.Weekdays, wd)
			}
		}
		for _, it := range rt.Items {
			e := s.matchExercise(ctx, uid, idx, it.Exercise)
			if e == nil {
				skipped = append(skipped, it.Exercise)
				continue
			}
			sets := min(max(it.Sets, 1), 10)
			d.Items = append(d.Items, draftItem{ExerciseID: e.ID, Exercise: e, Sets: sets, RepsMin: it.RepsMin, RepsMax: it.RepsMax, RestS: it.RestS, DurationS: it.DurationS, Notes: it.Notes})
		}
		if d.Name != "" && len(d.Items) > 0 {
			drafts = append(drafts, d)
		}
	}
	if len(drafts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "the AI's answer had no usable routine; try again or say the goal differently"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "routines": drafts, "skipped": skipped})
}

// handleAIParse understands a workout said in a sentence ("bench 3x8 at 80,
// then 3 sets of 10 pull-ups and a 20 minute run") as exercises with sets.
// Nothing is saved: the page shows it, the person corrects and logs it.
func (s *Server) handleAIParse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if len(in.Text) < 3 || len(in.Text) > 4000 {
		writeErr(w, http.StatusBadRequest, "write what you did in up to 4000 characters")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	cl, used, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": why})
		return
	}
	names, idx := s.exerciseNames(ctx, uid)
	prompt := fmt.Sprintf(`Today is %s. Their weights are usually in %s.
Their exercise list (name [kind]):
%s
What they did, in their words: """%s"""
Turn it into JSON only: {"name":"a short workout name","date":"YYYY-MM-DD or empty for today","duration_min":null,"exercises":[{"name":"exact name from the list, or their own words when nothing fits","kind":"weight|reps|time|cardio","sets":[{"weight_kg":80,"reps":8,"duration_s":null,"distance_m":null}]}]}
"3x8 at 80" is three sets of 8 reps at 80. Convert pounds to kg (1 lb = 0.4536 kg) when they say lb or their unit is lb. Bodyweight exercises have no weight. A run of 5 km in 25 minutes is one set with distance_m 5000 and duration_s 1500. Do not invent sets they did not mention.`,
		time.Now().Format("Monday 2 January 2006"), st.WeightUnit, names, in.Text)
	var out struct {
		Name        string   `json:"name"`
		Date        string   `json:"date"`
		DurationMin *float64 `json:"duration_min"`
		Exercises   []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
			Sets []struct {
				WeightKg  *float64 `json:"weight_kg"`
				Reps      *int     `json:"reps"`
				DurationS *int     `json:"duration_s"`
				DistanceM *float64 `json:"distance_m"`
			} `json:"sets"`
		} `json:"exercises"`
	}
	if err := cl.JSON(ctx, aiVoice, prompt, &out); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type pset struct {
		Weight    *float64 `json:"weight"`
		Reps      *int     `json:"reps"`
		DurationS *int     `json:"duration_s"`
		DistanceM *float64 `json:"distance_m"`
	}
	type pex struct {
		ExerciseID int64           `json:"exercise_id"`
		Exercise   *store.Exercise `json:"exercise"`
		Name       string          `json:"name"`
		Kind       string          `json:"kind"`
		New        bool            `json:"new"`
		Sets       []pset          `json:"sets"`
	}
	exs := []pex{}
	for _, e := range out.Exercises {
		p := pex{Name: strings.TrimSpace(e.Name), Kind: e.Kind, Sets: []pset{}}
		if p.Name == "" {
			continue
		}
		if m := s.matchExercise(ctx, uid, idx, p.Name); m != nil {
			p.ExerciseID, p.Exercise, p.Name, p.Kind = m.ID, m, m.Name, m.Kind
		} else {
			p.New = true
			if !store.ValidKind(p.Kind) {
				p.Kind = "weight"
			}
		}
		for _, x := range e.Sets {
			p.Sets = append(p.Sets, pset{Weight: x.WeightKg, Reps: x.Reps, DurationS: x.DurationS, DistanceM: x.DistanceM})
		}
		if len(p.Sets) == 0 {
			p.Sets = append(p.Sets, pset{})
		}
		exs = append(exs, p)
	}
	if len(exs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no exercise found in that; say what you did, like \"bench 3x8 at 80\""})
		return
	}
	date := out.Date
	if !validDate(date) || date == "" || date > store.Today() {
		date = store.Today()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": used, "name": strings.TrimSpace(out.Name), "date": date, "duration_min": out.DurationMin, "exercises": exs})
}
