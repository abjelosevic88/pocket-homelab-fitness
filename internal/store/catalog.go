package store

import "strings"

// CatalogExercise is one exercise of the built-in list. Names follow Strong
// and Hevy ("Bench Press (Barbell)") so their exports match by name.
type CatalogExercise struct {
	Key       string
	Name      string
	Muscle    string
	Secondary string
	Equipment string
	Kind      string
	Aliases   []string // what people type or say ("bench", "ohp")
}

// Muscles in the order the app lists them.
var Muscles = []string{"chest", "back", "shoulders", "biceps", "triceps", "forearms", "core", "quads", "hamstrings", "glutes", "calves", "full_body", "cardio", "other"}

// Equipment kinds.
var Equipment = []string{"barbell", "dumbbell", "machine", "cable", "bodyweight", "kettlebell", "band", "cardio", "other"}

// Kinds of exercise: what one set holds.
var Kinds = []string{"weight", "reps", "time", "cardio"}

// Catalog is the built-in list every account starts with.
var Catalog = []CatalogExercise{
	// chest
	{"bench-press", "Bench Press (Barbell)", "chest", "triceps,shoulders", "barbell", "weight", []string{"bench", "bench press", "flat bench"}},
	{"incline-bench-press", "Incline Bench Press (Barbell)", "chest", "shoulders,triceps", "barbell", "weight", []string{"incline bench", "incline press"}},
	{"bench-press-db", "Bench Press (Dumbbell)", "chest", "triceps,shoulders", "dumbbell", "weight", []string{"db bench", "dumbbell bench"}},
	{"incline-bench-press-db", "Incline Bench Press (Dumbbell)", "chest", "shoulders,triceps", "dumbbell", "weight", []string{"incline db press", "incline dumbbell press"}},
	{"chest-fly-db", "Chest Fly (Dumbbell)", "chest", "", "dumbbell", "weight", []string{"flyes", "flies", "db fly"}},
	{"cable-crossover", "Cable Crossover", "chest", "", "cable", "weight", []string{"cable fly", "crossover"}},
	{"chest-press-machine", "Chest Press (Machine)", "chest", "triceps", "machine", "weight", []string{"chest press", "machine press"}},
	{"push-up", "Push Up", "chest", "triceps,shoulders,core", "bodyweight", "reps", []string{"push ups", "pushups", "press ups"}},
	{"chest-dip", "Chest Dip", "chest", "triceps", "bodyweight", "reps", []string{"dips", "dip"}},
	// back
	{"deadlift", "Deadlift (Barbell)", "back", "hamstrings,glutes,forearms", "barbell", "weight", []string{"deadlift", "deadlifts", "dl"}},
	{"pull-up", "Pull Up", "back", "biceps", "bodyweight", "reps", []string{"pull ups", "pullups", "pull-ups"}},
	{"chin-up", "Chin Up", "back", "biceps", "bodyweight", "reps", []string{"chin ups", "chinups", "chin-ups"}},
	{"lat-pulldown", "Lat Pulldown (Cable)", "back", "biceps", "cable", "weight", []string{"lat pulldown", "pulldown", "lat pull"}},
	{"bent-over-row", "Bent Over Row (Barbell)", "back", "biceps", "barbell", "weight", []string{"barbell row", "bb row", "rows", "row"}},
	{"row-db", "Bent Over One Arm Row (Dumbbell)", "back", "biceps", "dumbbell", "weight", []string{"db row", "dumbbell row", "one arm row"}},
	{"seated-cable-row", "Seated Cable Row", "back", "biceps", "cable", "weight", []string{"cable row", "seated row"}},
	{"t-bar-row", "T Bar Row", "back", "biceps", "barbell", "weight", []string{"t-bar row"}},
	{"back-extension", "Back Extension", "back", "glutes,hamstrings", "bodyweight", "reps", []string{"hyperextension", "back extensions"}},
	{"face-pull", "Face Pull (Cable)", "shoulders", "back", "cable", "weight", []string{"face pulls"}},
	// shoulders
	{"overhead-press", "Overhead Press (Barbell)", "shoulders", "triceps", "barbell", "weight", []string{"ohp", "overhead press", "military press", "press"}},
	{"shoulder-press-db", "Shoulder Press (Dumbbell)", "shoulders", "triceps", "dumbbell", "weight", []string{"db shoulder press", "dumbbell press", "seated press"}},
	{"lateral-raise", "Lateral Raise (Dumbbell)", "shoulders", "", "dumbbell", "weight", []string{"lateral raises", "side raises", "laterals"}},
	{"front-raise", "Front Raise (Dumbbell)", "shoulders", "", "dumbbell", "weight", []string{"front raises"}},
	{"rear-delt-fly", "Reverse Fly (Dumbbell)", "shoulders", "back", "dumbbell", "weight", []string{"rear delt fly", "reverse fly"}},
	{"arnold-press", "Arnold Press (Dumbbell)", "shoulders", "triceps", "dumbbell", "weight", []string{"arnold press"}},
	{"shrug", "Shrug (Dumbbell)", "back", "forearms", "dumbbell", "weight", []string{"shrugs"}},
	// arms
	{"bicep-curl-bb", "Bicep Curl (Barbell)", "biceps", "forearms", "barbell", "weight", []string{"barbell curl", "bb curl"}},
	{"bicep-curl-db", "Bicep Curl (Dumbbell)", "biceps", "forearms", "dumbbell", "weight", []string{"curls", "curl", "db curl", "bicep curls"}},
	{"hammer-curl", "Hammer Curl (Dumbbell)", "biceps", "forearms", "dumbbell", "weight", []string{"hammer curls"}},
	{"preacher-curl", "Preacher Curl (Machine)", "biceps", "", "machine", "weight", []string{"preacher curl"}},
	{"cable-curl", "Bicep Curl (Cable)", "biceps", "", "cable", "weight", []string{"cable curl"}},
	{"triceps-pushdown", "Triceps Pushdown (Cable)", "triceps", "", "cable", "weight", []string{"pushdown", "tricep pushdown", "pushdowns"}},
	{"skullcrusher", "Skullcrusher (Barbell)", "triceps", "", "barbell", "weight", []string{"skull crushers", "skullcrushers", "lying triceps extension"}},
	{"triceps-extension-db", "Overhead Triceps Extension (Dumbbell)", "triceps", "", "dumbbell", "weight", []string{"overhead extension", "triceps extension"}},
	{"close-grip-bench", "Close Grip Bench Press (Barbell)", "triceps", "chest", "barbell", "weight", []string{"close grip bench", "cgbp"}},
	{"triceps-dip", "Triceps Dip", "triceps", "chest", "bodyweight", "reps", []string{"bench dips", "tricep dips"}},
	{"wrist-curl", "Wrist Curl (Barbell)", "forearms", "", "barbell", "weight", []string{"wrist curls"}},
	// legs
	{"squat", "Squat (Barbell)", "quads", "glutes,hamstrings,core", "barbell", "weight", []string{"squat", "squats", "back squat", "bb squat"}},
	{"front-squat", "Front Squat (Barbell)", "quads", "glutes,core", "barbell", "weight", []string{"front squats"}},
	{"goblet-squat", "Goblet Squat (Kettlebell)", "quads", "glutes", "kettlebell", "weight", []string{"goblet squat", "goblet squats"}},
	{"leg-press", "Leg Press (Machine)", "quads", "glutes", "machine", "weight", []string{"leg press"}},
	{"lunge-db", "Lunge (Dumbbell)", "quads", "glutes", "dumbbell", "weight", []string{"lunges", "lunge", "walking lunges"}},
	{"bulgarian-split-squat", "Bulgarian Split Squat", "quads", "glutes", "dumbbell", "weight", []string{"split squat", "bss", "bulgarians"}},
	{"leg-extension", "Leg Extension (Machine)", "quads", "", "machine", "weight", []string{"leg extensions", "leg extension"}},
	{"romanian-deadlift", "Romanian Deadlift (Barbell)", "hamstrings", "glutes,back", "barbell", "weight", []string{"rdl", "rdls", "romanian deadlift"}},
	{"leg-curl", "Seated Leg Curl (Machine)", "hamstrings", "", "machine", "weight", []string{"leg curl", "leg curls", "hamstring curl"}},
	{"lying-leg-curl", "Lying Leg Curl (Machine)", "hamstrings", "", "machine", "weight", []string{"lying leg curl"}},
	{"hip-thrust", "Hip Thrust (Barbell)", "glutes", "hamstrings", "barbell", "weight", []string{"hip thrust", "hip thrusts"}},
	{"glute-bridge", "Glute Bridge", "glutes", "hamstrings", "bodyweight", "reps", []string{"glute bridges", "bridge"}},
	{"kettlebell-swing", "Kettlebell Swing", "glutes", "hamstrings,core", "kettlebell", "weight", []string{"kb swing", "swings", "kettlebell swings"}},
	{"step-up", "Step Up (Dumbbell)", "quads", "glutes", "dumbbell", "weight", []string{"step ups"}},
	{"calf-raise", "Standing Calf Raise (Machine)", "calves", "", "machine", "weight", []string{"calf raise", "calf raises", "calves"}},
	{"seated-calf-raise", "Seated Calf Raise", "calves", "", "machine", "weight", []string{"seated calf raise"}},
	{"bodyweight-squat", "Squat (Bodyweight)", "quads", "glutes", "bodyweight", "reps", []string{"air squats", "bodyweight squats"}},
	// core
	{"plank", "Plank", "core", "", "bodyweight", "time", []string{"plank", "planks"}},
	{"side-plank", "Side Plank", "core", "", "bodyweight", "time", []string{"side planks"}},
	{"crunch", "Crunch", "core", "", "bodyweight", "reps", []string{"crunches", "sit ups", "situps"}},
	{"hanging-leg-raise", "Hanging Leg Raise", "core", "", "bodyweight", "reps", []string{"leg raises", "hanging leg raises"}},
	{"russian-twist", "Russian Twist", "core", "", "bodyweight", "reps", []string{"russian twists"}},
	{"ab-wheel", "Ab Wheel", "core", "", "other", "reps", []string{"ab rollout", "ab wheel rollout"}},
	{"cable-crunch", "Cable Crunch", "core", "", "cable", "weight", []string{"cable crunches"}},
	{"dead-bug", "Dead Bug", "core", "", "bodyweight", "reps", []string{"dead bugs"}},
	// full body
	{"burpee", "Burpee", "full_body", "", "bodyweight", "reps", []string{"burpees"}},
	{"clean-and-press", "Clean and Press (Barbell)", "full_body", "shoulders,quads", "barbell", "weight", []string{"clean and press", "clean & press"}},
	{"power-clean", "Power Clean (Barbell)", "full_body", "back,quads", "barbell", "weight", []string{"power clean", "cleans"}},
	{"thruster", "Thruster (Barbell)", "full_body", "quads,shoulders", "barbell", "weight", []string{"thrusters"}},
	{"farmers-walk", "Farmers Walk (Dumbbell)", "full_body", "forearms,core", "dumbbell", "cardio", []string{"farmers carry", "farmer's walk"}},
	{"jump-rope", "Jump Rope", "cardio", "calves", "other", "time", []string{"skipping", "jump rope"}},
	{"mountain-climber", "Mountain Climber", "core", "", "bodyweight", "time", []string{"mountain climbers"}},
	// cardio
	{"running", "Running", "cardio", "", "cardio", "cardio", []string{"run", "running", "jog", "jogging"}},
	{"treadmill", "Running (Treadmill)", "cardio", "", "cardio", "cardio", []string{"treadmill"}},
	{"cycling", "Cycling", "cardio", "", "cardio", "cardio", []string{"bike", "cycling", "ride"}},
	{"stationary-bike", "Cycling (Stationary Bike)", "cardio", "", "cardio", "cardio", []string{"exercise bike", "spin", "stationary bike"}},
	{"rowing-machine", "Rowing (Machine)", "cardio", "back", "cardio", "cardio", []string{"rower", "erg", "rowing"}},
	{"elliptical", "Elliptical Trainer", "cardio", "", "cardio", "cardio", []string{"elliptical", "cross trainer"}},
	{"stair-climber", "Stair Climber", "cardio", "", "cardio", "time", []string{"stairmaster", "stairs"}},
	{"walking", "Walking", "cardio", "", "cardio", "cardio", []string{"walk", "walking"}},
	{"swimming", "Swimming", "cardio", "full_body", "cardio", "cardio", []string{"swim", "swimming"}},
}

// CatalogByKey finds a built-in exercise.
func CatalogByKey(key string) (CatalogExercise, bool) {
	for _, c := range Catalog {
		if c.Key == key {
			return c, true
		}
	}
	return CatalogExercise{}, false
}

// NormName is a name for matching: lower case, single spaces, no
// punctuation around ("Pull-Ups" = "pull ups").
func NormName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	r := strings.NewReplacer("-", " ", "_", " ", "’", "'", ".", "", "  ", " ")
	s = r.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// CatalogMatch finds the built-in exercise a name means (its name, its name
// without the equipment, or an alias).
func CatalogMatch(name string) (CatalogExercise, bool) {
	n := NormName(name)
	if n == "" {
		return CatalogExercise{}, false
	}
	for _, c := range Catalog {
		if NormName(c.Name) == n {
			return c, true
		}
	}
	for _, c := range Catalog {
		for _, a := range c.Aliases {
			if NormName(a) == n {
				return c, true
			}
		}
	}
	for _, c := range Catalog {
		base := c.Name
		if i := strings.Index(base, " ("); i > 0 {
			base = base[:i]
		}
		if NormName(base) == n || NormName(base)+"s" == n {
			return c, true
		}
	}
	return CatalogExercise{}, false
}
