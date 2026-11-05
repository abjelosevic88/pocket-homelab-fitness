// Package pocket talks to sibling Pocket apps: Pocket Health (your weight,
// the runs and rides from your watch, finished workouts sent back), Pocket
// Habits (a training day ticks a habit), Pocket Calendar (planned sessions),
// Pocket Weather (the day for an outdoor session) and Pocket Music (a
// playlist to train to). Same bearer-token JSON API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// raw downloads a body as bytes (an attachment), at most limit bytes.
func (c *Client) raw(ctx context.Context, path string, q url.Values, limit int64) ([]byte, string, error) {
	if !c.Configured() {
		return nil, "", errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, "", errors.New("token rejected (401)")
	case res.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	case res.StatusCode >= 300:
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > limit {
		return nil, "", errors.New("too large")
	}
	return b, res.Header.Get("Content-Type"), nil
}

// ---------------- Pocket Health ----------------

// HealthWorkout is a workout in Pocket Health (a run from the watch).
type HealthWorkout struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	Date       string  `json:"date"`
	DurationS  int     `json:"duration_s"`
	DistanceM  float64 `json:"distance_m"`
	EnergyKcal float64 `json:"energy_kcal"`
	AvgHR      float64 `json:"avg_hr"`
	Source     string  `json:"source"`
}

// HealthWorkouts lists the person's workouts in Pocket Health between two dates.
func (c *Client) HealthWorkouts(ctx context.Context, from, to string) ([]HealthWorkout, error) {
	var raw json.RawMessage
	q := url.Values{"from": {from}, "to": {to}, "limit": {"500"}}
	if err := c.do(ctx, http.MethodGet, "/api/v1/workouts", q, nil, &raw); err != nil {
		return nil, err
	}
	return list[HealthWorkout](raw, "workouts")
}

// WeightPoint is one day's weight.
type WeightPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// Weight reads the person's weight (kg) over a number of days, oldest first.
func (c *Client) Weight(ctx context.Context, days int) ([]WeightPoint, error) {
	var out struct {
		Points []WeightPoint `json:"points"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/metrics/weight", url.Values{"range": {fmt.Sprint(days)}}, nil, &out); err != nil {
		return nil, err
	}
	if out.Points == nil {
		out.Points = []WeightPoint{}
	}
	return out.Points, nil
}

// NewHealthWorkout is a finished workout sent to Pocket Health. Health keeps
// one per start time, kind and app, so sending it again updates it.
type NewHealthWorkout struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
	DurationS  int     `json:"duration_s"`
	DistanceM  float64 `json:"distance_m,omitempty"`
	EnergyKcal float64 `json:"energy_kcal,omitempty"`
	Notes      string  `json:"notes"`
}

// SendWorkout records a workout in Pocket Health and returns its id there.
func (c *Client) SendWorkout(ctx context.Context, w NewHealthWorkout) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/workouts", nil, w, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// ---------------- Pocket Habits ----------------

// Habit is a Pocket Habits habit.
type Habit struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Habits lists the person's habits.
func (c *Client) Habits(ctx context.Context) ([]Habit, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/habits", nil, nil, &raw); err != nil {
		return nil, err
	}
	return list[Habit](raw, "habits")
}

// MarkHabit ticks a habit on a date (status 1 = done).
func (c *Client) MarkHabit(ctx context.Context, habitID int64, date string) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/habits/%d/entries/%s", habitID, date), nil, map[string]any{"status": 1}, nil)
}

// ---------------- Pocket Calendar ----------------

// Calendar is one of the person's calendars.
type Calendar struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Writable bool   `json:"writable"`
	Default  bool   `json:"is_default"`
}

// Calendars lists the calendars the person can write to.
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/calendars", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[Calendar](raw, "calendars")
	if err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, cal := range all {
		if cal.Writable {
			out = append(out, cal)
		}
	}
	return out, nil
}

// Event is a planned session on the calendar: from its time for its length,
// or all day when it has no time.
type Event struct {
	CalendarID  int64
	Title       string
	Description string
	URL         string
	Start       string // YYYY-MM-DDTHH:MM, or YYYY-MM-DD for all day
	Minutes     int
}

func (e Event) body() map[string]any {
	b := map[string]any{"title": e.Title, "description": e.Description, "url": e.URL, "categories": []string{"Fitness"}}
	start, err := time.Parse("2006-01-02T15:04", e.Start)
	if err != nil {
		b["all_day"], b["start"], b["busy"] = true, e.Start, false
	} else {
		if e.Minutes <= 0 {
			e.Minutes = 60
		}
		b["all_day"], b["start"], b["end"] = false, start.Format("2006-01-02T15:04"), start.Add(time.Duration(e.Minutes)*time.Minute).Format("2006-01-02T15:04")
	}
	if e.CalendarID > 0 {
		b["calendar_id"] = e.CalendarID
	}
	return b
}

// CreateEvent adds the event and returns its id.
func (c *Client) CreateEvent(ctx context.Context, e Event) (int64, error) {
	var out struct {
		EventID int64 `json:"event_id"`
		ID      any   `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/events", nil, e.body(), &out); err != nil {
		return 0, err
	}
	if out.EventID > 0 {
		return out.EventID, nil
	}
	// a bare {"id": 12} or an occurrence id like "12:20261104"
	var id int64
	_, _ = fmt.Sscan(strings.SplitN(fmt.Sprint(out.ID), ":", 2)[0], &id)
	return id, nil
}

// UpdateEvent moves or renames the event.
func (c *Client) UpdateEvent(ctx context.Context, id int64, e Event) error {
	b := e.body()
	delete(b, "calendar_id")
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/events/%d", id), nil, b, nil)
}

// DeleteEvent removes the event; one already gone counts as deleted.
func (c *Client) DeleteEvent(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/events/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ---------------- Pocket Weather ----------------

// WeatherDay is the weather of a day at home, in a sentence.
type WeatherDay struct {
	Date  string `json:"date"`
	Text  string `json:"text"`
	Place string `json:"place"`
	Day   struct {
		Code       int      `json:"code"`
		Condition  string   `json:"condition"`
		TMin       *float64 `json:"t_min"`
		TMax       *float64 `json:"t_max"`
		Precip     *float64 `json:"precip"`
		PrecipProb *float64 `json:"precip_prob"`
	} `json:"day"`
}

// Day reads the weather of a date at the person's home place.
func (c *Client) Day(ctx context.Context, date string) (*WeatherDay, error) {
	var out struct {
		WeatherDay
		Location *struct {
			Name string `json:"name"`
		} `json:"location"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &out); err != nil {
		return nil, err
	}
	if out.Location != nil {
		out.Place = out.Location.Name
	}
	return &out.WeatherDay, nil
}

// ---------------- Pocket Music ----------------

// Playlist is one of the person's playlists.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
}

// Playlists lists the person's playlists.
func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/playlists", nil, nil, &raw); err != nil {
		return nil, err
	}
	return list[Playlist](raw, "playlists")
}
