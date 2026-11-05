package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// exerciseID finds one of a person's exercises by name.
func (a *app) exerciseID(auth, name string) int64 {
	a.t.Helper()
	for _, x := range a.list(auth, "/api/v1/exercises") {
		e := x.(map[string]any)
		if e["name"] == name {
			return int64(num(e, "id"))
		}
	}
	a.t.Fatalf("no exercise %q", name)
	return 0
}

func bench(weight float64, reps ...int) map[string]any {
	var sets []any
	for _, r := range reps {
		sets = append(sets, map[string]any{"weight": weight, "reps": r})
	}
	return map[string]any{"name": "Bench Press (Barbell)", "sets": sets}
}

// Every exercise, routine, plan and workout belongs to one person; someone
// else's ids are 404, and an admin sees no more than anyone else.
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	benchID := a.exerciseID(a.alice, "Bench Press (Barbell)")
	own := a.expect(201, a.alice, "POST", "/api/v1/exercises", map[string]any{"name": "Sled push", "muscle": "quads", "equipment": "other", "kind": "cardio"})
	rt := a.expect(201, a.alice, "POST", "/api/v1/routines", map[string]any{"name": "Push day", "items": []any{map[string]any{"exercise_id": benchID, "sets": 3, "reps_min": 6, "reps_max": 8}}})
	a.ok(a.alice, "PUT", "/api/v1/plan", []any{map[string]any{"weekday": 1, "routine_id": num(rt, "id"), "time": "18:00"}})
	logged := a.expect(201, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"name": "Push day", "date": store.AddDays(store.Today(), -1), "exercises": []any{bench(80, 8, 8, 7)}})
	wid := itoa(int64(num(logged["workout"].(map[string]any), "id")))
	started := a.expect(201, a.alice, "POST", "/api/v1/workouts", map[string]any{"routine_id": num(rt, "id")})
	sid := itoa(int64(num(started["workout"].(map[string]any), "id")))

	// Bob has his own copy of the built-in list, and nothing of Alice's
	if a.exerciseID(a.bob, "Bench Press (Barbell)") == benchID {
		t.Fatal("bob shares alice's exercise row")
	}
	if l := a.list(a.bob, "/api/v1/routines"); len(l) != 0 {
		t.Fatalf("bob sees %d routines", len(l))
	}
	if l := a.list(a.bob, "/api/v1/workouts"); len(l) != 0 {
		t.Fatalf("bob sees %d workouts", len(l))
	}
	if l := a.list(a.bob, "/api/v1/plan"); len(l) != 0 {
		t.Fatalf("bob sees alice's plan: %v", l)
	}
	act := a.ok(a.bob, "GET", "/api/v1/workouts/active", nil)
	if act["workout"] != nil {
		t.Fatal("bob sees alice's workout in progress")
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/workouts/" + wid}, {"PUT", "/api/v1/workouts/" + wid}, {"DELETE", "/api/v1/workouts/" + wid},
		{"POST", "/api/v1/workouts/" + sid + "/finish"}, {"POST", "/api/v1/workouts/" + wid + "/resend"}, {"POST", "/api/v1/workouts/" + wid + "/routine"},
		{"GET", "/api/v1/routines/" + id(rt)}, {"PUT", "/api/v1/routines/" + id(rt)}, {"DELETE", "/api/v1/routines/" + id(rt)},
		{"GET", "/api/v1/exercises/" + id(own)}, {"PUT", "/api/v1/exercises/" + id(own)}, {"DELETE", "/api/v1/exercises/" + id(own)},
		{"GET", "/api/v1/exercises/" + id(own) + "/history"}, {"GET", "/api/v1/records?exercise=" + id(own)},
	} {
		a.expect(404, a.bob, c.method, c.path, map[string]any{"name": "Mine"})
	}
	// nor use Alice's exercises or routines in his own things
	a.expect(404, a.bob, "POST", "/api/v1/workouts/log", map[string]any{"exercises": []any{map[string]any{"exercise_id": benchID, "sets": []any{map[string]any{"weight": 50, "reps": 5}}}}})
	a.expect(404, a.bob, "POST", "/api/v1/workouts", map[string]any{"routine_id": num(rt, "id")})
	a.expect(404, a.bob, "POST", "/api/v1/routines", map[string]any{"name": "x", "items": []any{map[string]any{"exercise_id": benchID}}})
	a.expect(404, a.bob, "PUT", "/api/v1/plan", []any{map[string]any{"weekday": 2, "routine_id": num(rt, "id")}})
	bobStart := a.expect(201, a.bob, "POST", "/api/v1/workouts", map[string]any{})
	bsid := itoa(int64(num(bobStart["workout"].(map[string]any), "id")))
	a.expect(404, a.bob, "PUT", "/api/v1/workouts/"+bsid, map[string]any{"sets": []any{map[string]any{"exercise_id": benchID, "weight": 50, "reps": 5}}})

	// Alice (admin) sees none of Bob's either
	a.expect(404, a.alice, "GET", "/api/v1/workouts/"+bsid, nil)
	if wk := a.ok(a.bob, "GET", "/api/v1/summary", nil); num(wk, "week_done") != 0 {
		t.Fatalf("bob's week counts alice's workouts: %v", wk)
	}
	// counted per person
	var n int
	_ = a.st.DB.QueryRow(`SELECT COUNT(1) FROM workouts WHERE user_id = ?`, a.ids["Bob"]).Scan(&n)
	if n != 1 {
		t.Fatalf("bob has %d workouts", n)
	}
}

// A workout from a routine starts with last time's numbers, is saved set
// by set, and finishing it keeps only the ticked sets and finds records.
func TestWorkoutFlowAndRecords(t *testing.T) {
	a := newApp(t, nil)
	benchID := a.exerciseID(a.alice, "Bench Press (Barbell)")
	rt := a.expect(201, a.alice, "POST", "/api/v1/routines", map[string]any{"name": "Push day", "items": []any{map[string]any{"exercise_id": benchID, "sets": 3, "reps_max": 8, "weight": 60}}})

	// the first time: the routine's aim; no records on a first time
	first := a.expect(201, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"name": "Push day", "date": store.AddDays(store.Today(), -3), "routine_id": num(rt, "id"),
		"exercises": []any{map[string]any{"exercise_id": benchID, "sets": []any{map[string]any{"weight": 40, "reps": 8, "type": "warmup"}, map[string]any{"weight": 80, "reps": 8}, map[string]any{"weight": 80, "reps": 8}}}}})
	if recs := first["records"].([]any); len(recs) != 0 {
		t.Fatalf("a first time is no record: %v", recs)
	}
	fw := first["workout"].(map[string]any)
	if num(fw, "set_count") != 2 || num(fw, "volume") != 1280 {
		t.Fatalf("warm-ups do not count: %v sets, %v kg", fw["set_count"], fw["volume"])
	}

	// starting from the routine fills last time's work sets
	st := a.expect(201, a.alice, "POST", "/api/v1/workouts", map[string]any{"routine_id": num(rt, "id")})
	wk := st["workout"].(map[string]any)
	sets := wk["sets"].([]any)
	if len(sets) != 3 || num(sets[0].(map[string]any), "weight") != 80 || sets[0].(map[string]any)["done"] != false {
		t.Fatalf("sets from last time, not ticked: %v", sets)
	}
	if prev := st["previous"].(map[string]any)[itoa(benchID)].([]any); len(prev) != 3 {
		t.Fatalf("last time had 3 sets: %v", prev)
	}
	// a second start gives the same workout back
	again := a.expect(200, a.alice, "POST", "/api/v1/workouts", map[string]any{})
	if num(again["workout"].(map[string]any), "id") != num(wk, "id") {
		t.Fatal("one workout runs at a time")
	}
	wid := id(wk)
	a.expect(400, a.alice, "POST", "/api/v1/workouts/"+wid+"/finish", nil) // nothing ticked yet
	a.ok(a.alice, "PUT", "/api/v1/workouts/"+wid, map[string]any{"sets": []any{
		map[string]any{"exercise_id": benchID, "position": 0, "weight": 85, "reps": 6, "done": true},
		map[string]any{"exercise_id": benchID, "position": 0, "weight": 80, "reps": 8, "done": true},
		map[string]any{"exercise_id": benchID, "position": 0, "weight": 80, "reps": 8, "done": false},
	}})
	done := a.ok(a.alice, "POST", "/api/v1/workouts/"+wid+"/finish", map[string]any{"feel": 4})
	w2 := done["workout"].(map[string]any)
	if w2["ended_at"] == "" || num(w2, "set_count") != 2 || num(w2, "feel") != 4 {
		t.Fatalf("finished with the two ticked sets: %v", w2)
	}
	recs := done["records"].([]any)
	if len(recs) != 1 || !strings.Contains(recs[0].(map[string]any)["text"].(string), "85 kg × 6") {
		t.Fatalf("85 × 6 is the heaviest yet: %v", recs)
	}
	if links := done["links"].(map[string]any); links["health"].(map[string]any)["state"] != "off" {
		t.Fatalf("no Pocket Health here: %v", links)
	}
	a.expect(409, a.alice, "POST", "/api/v1/workouts/"+wid+"/finish", nil)
	if wv := done["week"].(map[string]any); num(wv, "done") < 1 {
		t.Fatalf("the week counts it: %v", wv)
	}
	all := a.ok(a.alice, "GET", "/api/v1/records?exercise="+itoa(benchID), nil)
	best := map[string]float64{}
	for _, r := range all["records"].([]any) {
		m := r.(map[string]any)
		best[m["kind"].(string)] = num(m, "value")
	}
	if best["weight"] != 85 || best["volume"] != 640 {
		t.Fatalf("records: %v", best)
	}
	// keep it as a routine
	kept := a.expect(201, a.alice, "POST", "/api/v1/workouts/"+wid+"/routine", map[string]any{"name": "Heavy bench"})
	if items := kept["items"].([]any); len(items) != 1 || num(items[0].(map[string]any), "sets") != 2 {
		t.Fatalf("the routine has the workout's exercise: %v", kept)
	}
	// the day in words, for Pocket Day
	day := a.ok(a.alice, "GET", "/api/v1/day/"+store.Today(), nil)
	if ws := day["workouts"].([]any); len(ws) != 1 || !strings.Contains(day["text"].(string), "Push day") {
		t.Fatalf("today: %v", day)
	}
}

// Logging by name finds the built-in exercise for what people type, and
// adds unknown ones as their own; a source_ref sent again updates.
func TestLogByName(t *testing.T) {
	a := newApp(t, nil)
	body := map[string]any{"source_ref": "voice-1", "exercises": []any{
		map[string]any{"name": "bench", "sets": []any{map[string]any{"weight": 70, "reps": 10}}},
		map[string]any{"name": "pull-ups", "sets": []any{map[string]any{"reps": 9}}},
		map[string]any{"name": "Sled push", "kind": "cardio", "sets": []any{map[string]any{"distance_m": 40, "duration_s": 30}}},
	}}
	res := a.expect(201, a.alice, "POST", "/api/v1/workouts/log", body)
	ex := res["exercises"].(map[string]any)
	names := map[string]bool{}
	for _, e := range ex {
		names[e.(map[string]any)["name"].(string)] = true
	}
	if !names["Bench Press (Barbell)"] || !names["Pull Up"] || !names["Sled push"] {
		t.Fatalf("matched names: %v", names)
	}
	again := a.expect(200, a.alice, "POST", "/api/v1/workouts/log", body)
	if num(again["workout"].(map[string]any), "id") != num(res["workout"].(map[string]any), "id") {
		t.Fatal("the same source_ref updates the workout")
	}
	if l := a.list(a.alice, "/api/v1/workouts"); len(l) != 1 {
		t.Fatalf("%d workouts after sending it twice", len(l))
	}
	a.expect(400, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"exercises": []any{}})
	a.expect(400, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"exercises": []any{map[string]any{"name": "bench", "sets": []any{map[string]any{"weight": 9000, "reps": 1}}}}})
}

// Strong's and Hevy's exports import, and importing again adds nothing; a
// Pocket Fitness export moves to another account the same way.
func TestImports(t *testing.T) {
	a := newApp(t, nil)
	strong := "Date;Workout Name;Duration;Exercise Name;Set Order;Weight;Reps;Distance;Seconds;Notes;Workout Notes;RPE\n" +
		"2026-10-01 18:00:00;Push;1h 5m;Bench Press (Barbell);W;40;10;0;0;;;\n" +
		"2026-10-01 18:00:00;Push;1h 5m;Bench Press (Barbell);1;80;8;0;0;;;8\n" +
		"2026-10-01 18:00:00;Push;1h 5m;Bench Press (Barbell);Rest Timer;0;0;0;90;;;\n" +
		"2026-10-01 18:00:00;Push;1h 5m;Overhead Press (Barbell);1;50;6;0;0;;;\n" +
		"2026-10-03 07:30:00;Run;30m;Running;1;0;0;5.2;1800;;;\n"
	hevy := "title,start_time,end_time,description,exercise_title,superset_id,exercise_notes,set_index,set_type,weight_kg,reps,distance_km,duration_seconds,rpe\n" +
		"Legs,\"8 Oct 2026, 17:30\",\"8 Oct 2026, 18:40\",,Squat (Barbell),,,0,warmup,60,5,,,\n" +
		"Legs,\"8 Oct 2026, 17:30\",\"8 Oct 2026, 18:40\",,Squat (Barbell),,,1,normal,100,5,,,8\n" +
		"Legs,\"8 Oct 2026, 17:30\",\"8 Oct 2026, 18:40\",,Plank,,,0,normal,,,,60,\n"
	post := func(body, q string) map[string]any {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import"+q, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.alice)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("import → %d %s", res.StatusCode, b)
		}
		_ = jsonUnmarshal(b, &m)
		return m
	}
	r1 := post(strong, "")
	if r1["format"] != "strong" || num(r1, "workouts") != 2 || num(r1, "sets") != 4 {
		t.Fatalf("strong: %v", r1)
	}
	r2 := post(strong, "")
	if num(r2, "workouts") != 0 || num(r2, "updated") != 2 {
		t.Fatalf("strong again: %v", r2)
	}
	h := post(hevy, "")
	if h["format"] != "hevy" || num(h, "workouts") != 1 {
		t.Fatalf("hevy: %v", h)
	}
	list := a.list(a.alice, "/api/v1/workouts?sets=1")
	if len(list) != 3 {
		t.Fatalf("%d workouts", len(list))
	}
	for _, x := range list {
		w := x.(map[string]any)
		if w["name"] == "Run" {
			s := w["sets"].([]any)[0].(map[string]any)
			if num(s, "distance_m") != 5200 || num(s, "duration_s") != 1800 {
				t.Fatalf("the run: %v", s)
			}
		}
		if w["name"] == "Legs" && num(w, "duration_s") != 70*60 {
			t.Fatalf("hevy's times: %v", w)
		}
	}

	// the JSON export into Bob's account, twice
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", bytes.NewReader(doc))
		req.Header.Set("Authorization", "Bearer "+a.bob)
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("json import %d: %v", i, err)
		}
		res.Body.Close()
	}
	if l := a.list(a.bob, "/api/v1/workouts"); len(l) != 3 {
		t.Fatalf("bob has %d workouts after importing twice", len(l))
	}
}

// Settings are per person; the box title only from an admin.
func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"week_goal": 5, "weight_unit": "lb", "title": "Gym"})
	bob := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if num(bob, "week_goal") != 3 || bob["weight_unit"] != "kg" || bob["title"] != "Gym" {
		t.Fatalf("bob's settings: goal %v unit %v title %v", bob["week_goal"], bob["weight_unit"], bob["title"])
	}
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"title": "Mine"})
	if s := a.ok(a.alice, "GET", "/api/v1/settings", nil); s["title"] != "Gym" || num(s, "week_goal") != 5 {
		t.Fatalf("a member changed the title or alice's goal: %v", s["title"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"week_goal": 40})
	// Core-only options are greyed, not hidden: the flags are always there
	if _, ok := bob["core_configured"]; !ok {
		t.Fatal("core_configured missing")
	}
	for _, slug := range []string{"health", "habits", "calendar", "weather", "music"} {
		if bob[slug+"_mode"] == nil {
			t.Fatalf("no connection mode for %s", slug)
		}
	}
}

// A Pocket Core link reaches only the routes of its scopes.
func TestLinkScopes(t *testing.T) {
	cases := []struct {
		method, path string
		scopes       []string
		want         bool
	}{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/summary", []string{"fitness:read"}, true},
		{"GET", "/api/v1/day/2026-11-05", []string{"fitness:read"}, true},
		{"GET", "/api/v1/workouts/12", []string{"fitness:read"}, true},
		{"GET", "/api/v1/exercises/3/history", []string{"fitness:read"}, true},
		{"GET", "/api/v1/summary", []string{"fitness:write"}, false},
		{"POST", "/api/v1/workouts/log", []string{"fitness:read"}, false},
		{"POST", "/api/v1/workouts/log", []string{"fitness:write"}, true},
		{"PUT", "/api/v1/workouts/4", []string{"fitness:write"}, true},
		{"POST", "/api/v1/workouts", []string{"fitness:write"}, false},
		{"POST", "/api/v1/workouts/4/finish", []string{"fitness:write"}, false},
		{"GET", "/api/v1/settings", []string{"fitness:read", "fitness:write"}, false},
		{"GET", "/api/v1/export.json", []string{"fitness:read"}, false},
		{"POST", "/api/v1/ai/review", []string{"fitness:read"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := linkAllows(c.scopes, r); got != c.want {
			t.Errorf("%s %s with %v: %v, want %v", c.method, c.path, c.scopes, got, c.want)
		}
	}
	// a linked app may change only the workouts it logged itself
	a := newApp(t, nil)
	mine := a.expect(201, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"exercises": []any{bench(60, 5)}})
	wid := int64(num(mine["workout"].(map[string]any), "id"))
	u, _ := a.st.GetUser(context.Background(), a.ids["Alice"])
	asApp := func(app, method string, body string) int {
		ctx := context.WithValue(context.WithValue(context.Background(), userKey, u), linkAppKey, app)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", itoa(wid))
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
		req := httptest.NewRequest(method, "/api/v1/workouts/"+itoa(wid), strings.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		if method == "DELETE" {
			a.s.handleDeleteWorkout(rec, req)
		} else {
			a.s.handleUpdateWorkout(rec, req)
		}
		return rec.Code
	}
	if code := asApp("voice", "PUT", `{"name":"x"}`); code != http.StatusForbidden {
		t.Fatalf("a linked app changed a workout it did not log: %d", code)
	}
	if code := asApp("voice", "DELETE", ""); code != http.StatusForbidden {
		t.Fatalf("a linked app deleted a workout it did not log: %d", code)
	}
	_, _ = a.st.DB.Exec(`UPDATE workouts SET source = 'voice' WHERE id = ?`, wid)
	if code := asApp("voice", "DELETE", ""); code != http.StatusNoContent {
		t.Fatalf("a linked app may delete its own workout: %d", code)
	}
}

// Weeks start on the person's day.
func TestWeekOf(t *testing.T) {
	for _, c := range []struct {
		d     string
		start int
		want  string
	}{{"2026-11-05", 1, "2026-11-02"}, {"2026-11-02", 1, "2026-11-02"}, {"2026-11-08", 1, "2026-11-02"}, {"2026-11-05", 7, "2026-11-01"}, {"2026-11-07", 6, "2026-11-07"}} {
		if got := store.WeekOf(c.d, c.start); got != c.want {
			t.Errorf("WeekOf(%s, %d) = %s, want %s", c.d, c.start, got, c.want)
		}
	}
}
