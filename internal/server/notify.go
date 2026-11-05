package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-fitness/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// pushes runs every minute: once a day, at each person's own hour, a
// message on the morning of a planned session ("fitness.plan_today" on the
// Core bus), and on the first day of the week how the last one went.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil {
			continue
		}
		if now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.morning(ctx, uid, st, false)
		if store.WeekOf(store.Today(), st.WeekStart) == store.Today() {
			s.weekReport(ctx, uid, st, false)
		}
	}
}

// morning tells a person about today's planned session (force: for "Send
// it now", sent even when switched off). It returns the text.
func (s *Server) morning(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	today := store.Today()
	plan := s.todayPlan(ctx, uid, today)
	if plan == nil {
		return "No session planned for today: a rest day.", false
	}
	r := plan["routine"].(*store.Routine)
	if done, _ := plan["done"].(bool); done && !force {
		return "", false
	}
	ex, _ := s.store.ExercisesByID(ctx, uid)
	var names []string
	for _, it := range r.Items {
		if e := ex[it.ExerciseID]; e != nil {
			names = append(names, e.Name)
		}
	}
	at := ""
	if t, _ := plan["time"].(string); t != "" {
		at = " at " + t
	}
	title := fmt.Sprintf("Today: %s%s", r.Name, at)
	lines := []string{fmt.Sprintf("About %d minutes, %d %s: %s.", plan["minutes"], len(names), plural(len(names), "exercise", "exercises"), strings.Join(names, ", "))}
	if wd, ok := plan["weather"]; ok && wd != nil {
		if w := s.weatherFor(ctx, uid, today); w != nil && w.Text != "" {
			lines = append(lines, "Weather: "+w.Text)
		}
	}
	wv := s.week(ctx, uid, today)
	if wv.Goal > 0 {
		lines = append(lines, fmt.Sprintf("This week so far: %d of %d.", wv.Done, wv.Goal))
	}
	body := strings.Join(lines, "\n")
	if !force {
		s.publish(ctx, uid, "fitness.plan_today", "plan:"+today, map[string]any{"date": today, "routine": r.Name, "routine_id": r.ID, "time": plan["time"],
			"minutes": plan["minutes"], "exercises": names, "text": title + ". " + lines[0], "url": s.appLink("/")})
	}
	if (!st.NotifyPlan && !force) || st.NotifyURL == "" {
		return title + "\n" + body, false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"muscle"}, Link: s.appLink("/")}); err != nil {
		s.log.Warn("notify", "user", uid, "err", err)
		return title + "\n" + body, false
	}
	return title + "\n" + body, true
}

// weekReport sums up last week: workouts against the goal, sets, weight
// lifted, records.
func (s *Server) weekReport(ctx context.Context, uid int64, st store.Settings, force bool) (string, bool) {
	last := store.AddDays(store.WeekOf(store.Today(), st.WeekStart), -7)
	wv := s.week(ctx, uid, last)
	list, _ := s.store.ListWorkouts(ctx, uid, store.WorkoutFilter{From: wv.Start, To: wv.End, Finished: true, Limit: 100})
	if wv.Done == 0 && !force {
		return "", false
	}
	sets, recs := 0, 0
	vol := 0.0
	for _, w := range list {
		sets += w.SetCount
		recs += w.Records
		vol += w.Volume
	}
	title := fmt.Sprintf("Last week: %d %s", wv.Done, plural(wv.Done, "workout", "workouts"))
	if wv.Goal > 0 {
		title = fmt.Sprintf("Last week: %d of %d workouts", wv.Done, wv.Goal)
		if wv.Done >= wv.Goal {
			title += " — goal met"
		}
	}
	parts := []string{fmt.Sprintf("%d sets", sets)}
	if vol > 0 {
		parts = append(parts, kgWords(vol, st.WeightUnit)+" lifted")
	}
	if recs > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", recs, plural(recs, "record", "records")))
	}
	body := strings.Join(parts, ", ") + "."
	if n := s.streak(ctx, uid); n > 1 {
		body += fmt.Sprintf(" %d weeks in a row on goal.", n)
	}
	if (!st.NotifyWeek && !force) || st.NotifyURL == "" {
		return title + "\n" + body, false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "low", Tags: []string{"muscle"}, Link: s.appLink("/progress")}); err != nil {
		s.log.Warn("notify", "user", uid, "err", err)
		return title + "\n" + body, false
	}
	return title + "\n" + body, true
}

// kgWords is a total weight in words: "12,340 kg", "27,205 lb".
func kgWords(kg float64, unit string) string {
	v, u := kg, "kg"
	if unit == "lb" {
		v, u = kg*2.20462, "lb"
	}
	n := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, c := range n {
		if i > 0 && (len(n)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String() + " " + u
}

// once reports true the first time a key is seen.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, ok := s.store.CacheGet(ctx, "once:"+key); ok {
		return false
	}
	_ = s.store.CachePut(ctx, "once:"+key, "1")
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Fitness", Body: notifyTestBody, Priority: "info", Tags: []string{"muscle"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCheckNow sends today's message (or last week's with "week") now and
// says what it said.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	var in struct {
		Which string `json:"which"`
	}
	_ = decode(r, &in)
	var body string
	var sent bool
	if in.Which == "week" {
		body, sent = s.weekReport(ctx, uid, st, true)
	} else {
		body, sent = s.morning(ctx, uid, st, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "body": body})
}

const notifyTestBody = "Notifications work. The morning of a planned session, and on Mondays how last week went, will arrive here."
