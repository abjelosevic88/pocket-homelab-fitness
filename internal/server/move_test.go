package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-fitness/internal/config"
)

// moveLinks starts Fitness behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "fitness:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"fitness","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) send(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) moveImport(tok, ctype string, data []byte) map[string]any {
	a.t.Helper()
	code, out := a.send(tok, "POST", "/api/v1/import", ctype, data)
	if code != 200 {
		a.t.Fatalf("import → %d %s", code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{exp, "GET", "/api/v1/users", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{imp, "POST", "/api/v1/routines", 403},
		{imp, "POST", "/api/v1/workouts/log", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := a.send(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.send(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 2 || p.Exports[0].Kind != core.KindPocket || p.Imports[1].ContentType != "text/csv" {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's training to Bob: each import adds once, and nothing of
// one person shows up for the other.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	a.expect(201, a.alice, "POST", "/api/v1/workouts/log", map[string]any{"name": "Morning push", "exercises": []any{
		map[string]any{"name": "bench", "sets": []any{map[string]any{"weight": 70, "reps": 10}}},
	}})

	code, export := a.send("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	if code != 200 || !strings.Contains(string(export), "Morning push") {
		t.Fatalf("alice's export: %d %s", code, export)
	}
	if _, bob := a.send("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(bob), "Morning push") {
		t.Fatalf("bob's export holds alice's workouts: %s", bob)
	}
	if _, csv := a.send("pcx_alice_export", "GET", "/api/v1/export.csv", "", nil); !strings.Contains(string(csv), "Morning push") {
		t.Fatalf("alice's csv: %s", csv)
	}

	first := a.moveImport("pcx_bob_import", "application/json", export)
	if num(first, "workouts") != 1 {
		t.Fatalf("first import: %v", first)
	}
	second := a.moveImport("pcx_bob_import", "application/json", export)
	if num(second, "workouts")+num(second, "routines")+num(second, "exercises_added") != 0 {
		t.Fatalf("second import added: %v", second)
	}

	strong := "Date;Workout Name;Duration;Exercise Name;Set Order;Weight;Reps;Distance;Seconds;Notes;Workout Notes;RPE\n" +
		"2026-10-01 18:00:00;Push;1h 5m;Bench Press (Barbell);1;80;8;0;0;;;8\n"
	c1 := a.moveImport("pcx_bob_import", "text/csv", []byte(strong))
	c2 := a.moveImport("pcx_bob_import", "text/csv", []byte(strong))
	if num(c1, "workouts") != 1 || num(c2, "workouts") != 0 {
		t.Fatalf("csv imports: %v then %v", c1, c2)
	}

	if l := a.list(a.bob, "/api/v1/workouts"); len(l) != 2 {
		t.Fatalf("bob has %d workouts", len(l))
	}
	if l := a.list(a.alice, "/api/v1/workouts"); len(l) != 1 {
		t.Fatalf("alice's workouts changed: %d", len(l))
	}
}
