package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Fitness as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "fitness:read", Name: "See your training", Description: "Read your workouts with their exercises and sets, your routines and weekly plan, your records and how your week is going — for your journal, your health app, dashboards and Pocket AI."},
	{Scope: "fitness:write", Name: "Log workouts", Description: "Log a finished workout with its exercises and sets (\"I did 3×8 bench at 80 kg\") and change or remove the workouts that app logged itself."},
}

// portable is what Pocket Move may copy out and bring in for a person
// (docs/move.md in pocket-homelab-core); Register adds move:export/import.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "fitness.json", Kind: core.KindPocket, Title: "Your exercises, routines, weekly plan and every workout with its sets"},
		{Path: "/api/v1/export.csv", File: "workouts.csv", Kind: core.KindCSV, Title: "Every set of every workout as a table (Hevy's columns, weights in kg)"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, ContentType: "application/json", Title: "Adds the exercises, routines and workouts from another box; what you already have is left alone"},
		{Path: "/api/v1/import", Kind: "workouts-csv", ContentType: "text/csv", Title: "Adds the workouts from a Strong or Hevy CSV export; workouts already brought in are not added twice"},
	},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(overview|summary|week|progress|records|workouts|routines|plan|exercises|day/\d{4}-\d{2}-\d{2})$`), "fitness:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(workouts|routines|exercises)/\d+$`), "fitness:read"},
	{"GET", regexp.MustCompile(`^/api/v1/exercises/\d+/history$`), "fitness:read"},
	{"POST", regexp.MustCompile(`^/api/v1/workouts/log$`), "fitness:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/workouts/\d+$`), "fitness:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "fitness" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
