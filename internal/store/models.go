package store

import "strings"

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Exercise is one exercise a person picks from.
type Exercise struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	Name      string `json:"name"`
	Builtin   string `json:"builtin"`
	Muscle    string `json:"muscle"`
	Secondary string `json:"secondary"`
	Equipment string `json:"equipment"`
	Kind      string `json:"kind"` // weight | reps | time | cardio
	Notes     string `json:"notes"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// Filled on the way out (lists).
	Sets     int    `json:"sets,omitempty"`      // sets ever done
	LastDone string `json:"last_done,omitempty"` // date
}

// Routine is a planned session: exercises with what to aim for.
type Routine struct {
	ID        int64         `json:"id"`
	UserID    int64         `json:"-"`
	Name      string        `json:"name"`
	Notes     string        `json:"notes"`
	Outdoors  bool          `json:"outdoors"`
	Sort      int           `json:"sort"`
	Source    string        `json:"source"`
	Items     []RoutineItem `json:"items"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`

	// Filled on the way out.
	LastDone string `json:"last_done,omitempty"`
	Times    int    `json:"times,omitempty"`
	Weekdays []int  `json:"weekdays,omitempty"`
}

// RoutineItem is one exercise of a routine.
type RoutineItem struct {
	ID         int64    `json:"id"`
	ExerciseID int64    `json:"exercise_id"`
	Position   int      `json:"position"`
	Sets       int      `json:"sets"`
	RepsMin    *int     `json:"reps_min"`
	RepsMax    *int     `json:"reps_max"`
	Weight     *float64 `json:"weight"`
	DurationS  *int     `json:"duration_s"`
	DistanceM  *float64 `json:"distance_m"`
	RestS      *int     `json:"rest_s"`
	Notes      string   `json:"notes"`

	// Filled on the way out.
	Exercise *Exercise `json:"exercise,omitempty"`
}

// PlanDay is one day of the week with its routine.
type PlanDay struct {
	Weekday   int    `json:"weekday"` // 1 = Monday … 7 = Sunday
	RoutineID int64  `json:"routine_id"`
	Time      string `json:"time"`

	// Filled on the way out.
	Routine string `json:"routine,omitempty"`
}

// Workout is one session.
type Workout struct {
	ID          int64    `json:"id"`
	UserID      int64    `json:"-"`
	Name        string   `json:"name"`
	RoutineID   *int64   `json:"routine_id"`
	StartedAt   string   `json:"started_at"`
	EndedAt     string   `json:"ended_at"`
	Date        string   `json:"date"`
	DurationS   int      `json:"duration_s"`
	Notes       string   `json:"notes"`
	Feel        int      `json:"feel"`
	Bodyweight  *float64 `json:"bodyweight"`
	Source      string   `json:"source"`
	SourceRef   string   `json:"source_ref"`
	HealthRef   string   `json:"health_ref"`
	HealthError string   `json:"health_error"`
	HabitDone   string   `json:"habit_done"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`

	Sets []Set `json:"sets,omitempty"`

	// Totals, filled on the way out.
	Exercises int     `json:"exercises"`
	SetCount  int     `json:"set_count"`
	Volume    float64 `json:"volume"` // kg lifted: weight × reps of the done, non-warm-up sets
	Reps      int     `json:"reps"`
	DistanceM float64 `json:"distance_m"`
	Records   int     `json:"records"`
}

// InProgress reports whether the workout has not been finished yet.
func (w Workout) InProgress() bool { return w.EndedAt == "" }

// Set is one set of one exercise in a workout.
type Set struct {
	ID         int64    `json:"id"`
	ExerciseID int64    `json:"exercise_id"`
	Position   int      `json:"position"`
	SetNo      int      `json:"set_no"`
	Type       string   `json:"type"` // normal | warmup | drop | failure
	Weight     *float64 `json:"weight"`
	Reps       *int     `json:"reps"`
	DurationS  *int     `json:"duration_s"`
	DistanceM  *float64 `json:"distance_m"`
	RPE        *float64 `json:"rpe"`
	Done       bool     `json:"done"`
	Note       string   `json:"note"`
	RestS      *int     `json:"rest_s"`

	// Filled on the way out: the records this set set ("weight", "e1rm", …).
	PR []string `json:"pr,omitempty"`
}

// Counts reports whether a set counts toward totals and records.
func (s Set) Counts() bool { return s.Done && s.Type != "warmup" }

// Volume is weight × reps (0 for anything else).
func (s Set) Volume() float64 {
	if s.Weight == nil || s.Reps == nil {
		return 0
	}
	return *s.Weight * float64(*s.Reps)
}

// E1RM is the estimated one-rep max (Epley), for 1–12 reps.
func (s Set) E1RM() float64 {
	if s.Weight == nil || s.Reps == nil || *s.Reps < 1 || *s.Reps > 12 || *s.Weight <= 0 {
		return 0
	}
	if *s.Reps == 1 {
		return *s.Weight
	}
	return *s.Weight * (1 + float64(*s.Reps)/30)
}

// Record is a person's best at one exercise in one way.
type Record struct {
	ExerciseID int64   `json:"exercise_id"`
	Exercise   string  `json:"exercise"`
	Kind       string  `json:"kind"` // weight | e1rm | reps | volume | duration | distance
	Value      float64 `json:"value"`
	Weight     float64 `json:"weight,omitempty"` // the set behind it
	Reps       int     `json:"reps,omitempty"`
	WorkoutID  int64   `json:"workout_id"`
	Date       string  `json:"date"`
}

// Conn is how a person reaches one sibling Pocket app: "core" (an app link
// through Pocket Core), "direct" (URL + token) or "off"; "" = not chosen yet.
type Conn struct {
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	PublicURL string `json:"public_url"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Units for display (the data is always kilograms and metres).
	WeightUnit   string `json:"weight_unit"`   // kg | lb
	DistanceUnit string `json:"distance_unit"` // km | mi

	// Training.
	WeekGoal     int     `json:"week_goal"`     // workouts a week
	RestS        int     `json:"rest_s"`        // default rest between sets
	BodyweightKg float64 `json:"bodyweight_kg"` // when Pocket Health has no weight
	Plates       bool    `json:"plates"`        // show the plates for a barbell weight
	BarKg        float64 `json:"bar_kg"`

	// Notifications.
	NotifyPlan   bool   `json:"notify_plan"` // the morning of a planned session
	NotifyWeek   bool   `json:"notify_week"` // Monday: how last week went
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// The other Pocket apps, each switchable.
	HealthSend     bool   `json:"health_send"`   // a finished workout goes to Pocket Health
	HealthCount    bool   `json:"health_count"`  // runs, rides and walks from Pocket Health count toward the week
	HealthWeight   bool   `json:"health_weight"` // your weight from Pocket Health
	HabitID        int64  `json:"habit_id"`      // the Pocket Habits habit ticked on a workout day (0 = none)
	HabitName      string `json:"habit_name"`
	CalendarPlan   bool   `json:"calendar_plan"` // planned sessions on Pocket Calendar
	CalendarID     int64  `json:"calendar_id"`
	OutdoorWeather bool   `json:"outdoor_weather"` // the weather for outdoor sessions
	PlaylistID     string `json:"playlist_id"`     // a Pocket Music playlist to train to
	PlaylistName   string `json:"playlist_name"`

	// AI: your training in plain words, routines and quick logging.
	AIReview bool `json:"ai_review"`

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// Pocket AI through Core, or an OpenAI-compatible API with the person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// SplitTags turns "a, b ,c" into clean lowercase tags.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is the stored form of tags.
func JoinTags(s string) string { return strings.Join(SplitTags(s), ", ") }
