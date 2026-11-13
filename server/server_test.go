package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setup(t testing.TB) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	f, e := os.Open(root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	s, e := newStore(t.TempDir(), map[string]*FileRoot{"test": {RootConfig{ID: "test", Name: "Test", Path: root}, f}}, 4)
	if e != nil {
		t.Fatal(e)
	}
	return s, root
}
func write(t testing.TB, p, text string) {
	t.Helper()
	if e := os.WriteFile(p, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
}
func change(t testing.TB, s *Store, from, to string) Change {
	t.Helper()
	d, e := openDir(s.roots["test"], "")
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	st, e := statAt(d, from)
	if e != nil {
		t.Fatal(e)
	}
	return Change{From: from, To: to, Fingerprint: identity(&st)}
}
func finish(t testing.TB, s *Store, id string) *Job {
	t.Helper()
	until := time.Now().Add(20 * time.Second)
	for time.Now().Before(until) {
		s.mu.Lock()
		j := s.jobs[id]
		active := s.active
		s.mu.Unlock()
		if !active {
			return j
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job timeout")
	return nil
}
func TestRenameAndUndo(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a.txt"), "content")
	plan, e := s.createPlan("test", "", "owner", []Change{change(t, s, "a.txt", "b.txt")})
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.start(plan.ID, "owner")
	if e != nil {
		t.Fatal(e)
	}
	j = finish(t, s, j.ID)
	if j.Done != 1 || j.Failed != 0 {
		t.Fatalf("bad job %+v", j)
	}
	b, _ := os.ReadFile(filepath.Join(p, "b.txt"))
	if string(b) != "content" {
		t.Fatal("content changed")
	}
	undo, e := s.undo(j.ID, "owner")
	if e != nil {
		t.Fatal(e)
	}
	u, e := s.start(undo.ID, "owner")
	if e != nil {
		t.Fatal(e)
	}
	if finish(t, s, u.ID).Done != 1 {
		t.Fatal("undo failed")
	}
	if _, e = os.Stat(filepath.Join(p, "a.txt")); e != nil {
		t.Fatal(e)
	}
}
func TestNeverOverwriteAfterPreview(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "source")
	plan, e := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	if e != nil {
		t.Fatal(e)
	}
	write(t, filepath.Join(p, "b"), "new arrival")
	j, e := s.start(plan.ID, "x")
	if e != nil {
		t.Fatal(e)
	}
	j = finish(t, s, j.ID)
	if j.Failed != 1 {
		t.Fatal("expected conflict")
	}
	for name, text := range map[string]string{"a": "source", "b": "new arrival"} {
		b, _ := os.ReadFile(filepath.Join(p, name))
		if string(b) != text {
			t.Fatal("file overwritten")
		}
	}
}
func TestRejectChangedSource(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "old")
	plan, _ := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	write(t, filepath.Join(p, "a"), "changed content")
	j, e := s.start(plan.ID, "x")
	if e != nil {
		t.Fatal(e)
	}
	if finish(t, s, j.ID).Failed != 1 {
		t.Fatal("changed source accepted")
	}
}
func TestUndoRefusesReplacement(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "x")
	plan, _ := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	j, _ := s.start(plan.ID, "x")
	finish(t, s, j.ID)
	write(t, filepath.Join(p, "a"), "new arrival")
	if _, e := s.undo(j.ID, "x"); e == nil {
		t.Fatal("undo should conflict")
	}
}
func TestRootBoundary(t *testing.T) {
	s, p := setup(t)
	os.Symlink(t.TempDir(), filepath.Join(p, "outside"))
	for _, path := range []string{"../", "/etc", "outside", "a/../.."} {
		d, e := openDir(s.roots["test"], path)
		if e == nil {
			d.Close()
			t.Fatalf("escaped: %s", path)
		}
	}
}
func TestScanCanRepeat(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	for i := 0; i < 3; i++ {
		d, _ := openDir(s.roots["test"], "")
		entries, e := scan(d)
		d.Close()
		if e != nil || len(entries) != 1 {
			t.Fatal("scan cursor leaked")
		}
	}
}
func TestPlanValidation(t *testing.T) {
	s, p := setup(t)
	for _, n := range []string{"a", "b"} {
		write(t, filepath.Join(p, n), n)
	}
	for name, items := range map[string][]Change{"occupied": {change(t, s, "a", "b")}, "duplicate": {change(t, s, "a", "c"), change(t, s, "b", "c")}, "traversal": {change(t, s, "a", "../c")}, "case": {change(t, s, "a", "A")}, "same": {change(t, s, "a", "a")}} {
		t.Run(name, func(t *testing.T) {
			if _, e := s.createPlan("test", "", "x", items); e == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
}
func TestPlanIsBoundAndSingleUse(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	plan, _ := s.createPlan("test", "", "alice", []Change{change(t, s, "a", "b")})
	if _, e := s.start(plan.ID, "bob"); e == nil {
		t.Fatal("wrong owner accepted")
	}
	j, _ := s.start(plan.ID, "alice")
	finish(t, s, j.ID)
	if _, e := s.start(plan.ID, "alice"); e == nil {
		t.Fatal("plan reused")
	}
}
func TestExpiredPlan(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	plan, _ := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	plan.expires = time.Now().Add(-time.Second)
	if _, e := s.start(plan.ID, "x"); e == nil {
		t.Fatal("expired plan accepted")
	}
}
func TestCancellation(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	plan, _ := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	s.mu.Lock()
	j := &Job{ID: uid(), Root: "test", Owner: "x", State: "running", Total: 1, Items: plan.Items, stop: true}
	s.jobs[j.ID] = j
	s.active = true
	d, _ := openDir(s.roots["test"], "")
	go s.run(j, nativeDirectory{d})
	s.mu.Unlock()
	if finish(t, s, j.ID).Done != 0 {
		t.Fatal("stopped item was executed")
	}
}
func TestHistorySurvivesRestart(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	plan, _ := s.createPlan("test", "", "x", []Change{change(t, s, "a", "b")})
	j, _ := s.start(plan.ID, "x")
	finish(t, s, j.ID)
	reloaded, e := newStore(s.data, s.roots, 4)
	if e != nil {
		t.Fatal(e)
	}
	if reloaded.jobs[j.ID].Done != 1 {
		t.Fatal("history missing")
	}
}
func TestHTTPGates(t *testing.T) {
	s, _ := setup(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"code": true, "is_login": true, "data": []any{}})
	}))
	defer upstream.Close()
	a := &App{store: s, mode: "tos", platform: platformClient(), platformBase: upstream.URL}
	for _, tc := range []struct {
		name, cookie, csrf, origin string
		want                       int
	}{{"none", "", "", "", 401}, {"cookie-only", "TMSESSNAME=s; X-Csrf-Token=c", "", "", 401}, {"mismatch", "TMSESSNAME=s; X-Csrf-Token=c", "x", "", 401}, {"cross-origin", "TMSESSNAME=s; X-Csrf-Token=c", "c", "http://evil", 403}, {"valid", "TMSESSNAME=s; X-Csrf-Token=c", "c", "http://nas:8181", 200}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://nas:8181/v2/proxy/batch-rename/api/info", nil)
			r.Header.Set("Cookie", tc.cookie)
			r.Header.Set("X-Csrf-Token", tc.csrf)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			a.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d", w.Code)
			}
			if w.Header().Get("X-Batch-Rename-Version") != appVersion {
				t.Fatal("missing response build version")
			}
			if tc.want == 401 && !strings.Contains(w.Body.String(), `"AUTH_HEADERS_INVALID"`) {
				t.Fatal("missing application auth diagnostic")
			}
			if tc.want == 200 && !strings.Contains(w.Body.String(), `"version":"`+appVersion+`"`) {
				t.Fatal("missing info build version")
			}
		})
	}
}
func TestHTTPRejectUnknownBodyAndRoot(t *testing.T) {
	s, _ := setup(t)
	a := &App{store: s, mode: "demo"}
	for _, body := range []string{`{"root":"other","path":"","items":[]}`, `{"root":"test","extra":true}`} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:18946/api/plans", strings.NewReader(body))
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatal("invalid request accepted")
		}
	}
}
func BenchmarkScan5000(b *testing.B) {
	s, p := setup(b)
	for i := 0; i < 5000; i++ {
		write(b, filepath.Join(p, fmt.Sprintf("IMG_%06d.jpg", i)), "test")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d, _ := openDir(s.roots["test"], "")
		entries, e := scan(d)
		d.Close()
		if e != nil || len(entries) != 5000 {
			b.Fatal(e)
		}
	}
}

func TestHistoryRotatesCompletedAndKeepsInterrupted(t *testing.T) {
	s, _ := setup(t)
	ids := []string{}
	for i := 0; i < 32; i++ {
		j := &Job{ID: uid(), State: "done", Started: time.Unix(int64(i), 0).UTC().Format(time.RFC3339)}
		if i == 0 {
			j.State = "interrupted"
		}
		if err := s.save(j); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(s.data, "history", j.ID+".jsonl"), "journal")
		s.jobs[j.ID] = j
		ids = append(ids, j.ID)
	}
	if err := s.makeHistoryRoom(); err != nil {
		t.Fatal(err)
	}
	if len(s.jobs) != 31 || s.jobs[ids[0]] == nil || s.jobs[ids[1]] != nil {
		t.Fatal("wrong records rotated")
	}
	for _, ext := range []string{".json", ".jsonl"} {
		if _, err := os.Stat(filepath.Join(s.data, "history", ids[1]+ext)); !os.IsNotExist(err) {
			t.Fatal("old record retained")
		}
		if _, err := os.Stat(filepath.Join(s.data, "history", ids[0]+ext)); err != nil {
			t.Fatal("interrupted record removed")
		}
	}
}
func TestNoSecretsInJournal(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	plan, _ := s.createPlan("test", "", "hashed-owner", []Change{change(t, s, "a", "b")})
	j, _ := s.start(plan.ID, "hashed-owner")
	finish(t, s, j.ID)
	b, e := os.ReadFile(filepath.Join(s.data, "history", j.ID+".jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var c map[string]any
		if json.Unmarshal([]byte(l), &c) != nil {
			t.Fatal("bad journal")
		}
		if c["owner"] != nil {
			t.Fatal("owner leaked")
		}
	}
}

func TestRecycleProtected(t *testing.T) {
	s, p := setup(t)
	os.Mkdir(filepath.Join(p, "#recycle"), 0700)
	d, _ := openDir(s.roots["test"], "")
	defer d.Close()
	entries, e := scan(d)
	if e != nil || len(entries) != 0 || validName("#recycle") {
		t.Fatal("recycle directory should be protected")
	}
}

func TestInterruptedRecovery(t *testing.T) {
	s, p := setup(t)
	write(t, filepath.Join(p, "a"), "a")
	c := change(t, s, "a", "b")
	c.State = "pending"
	j := &Job{ID: uid(), Root: "test", Owner: "owner", State: "running", Total: 1, Items: []Change{c}}
	if err := s.save(j); err != nil {
		t.Fatal(err)
	}
	c.State = "intent"
	b, _ := json.Marshal(c)
	write(t, filepath.Join(s.data, "history", j.ID+".jsonl"), string(b)+"\n")
	recovered, err := newStore(s.data, s.roots, 4)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.active || recovered.jobs[j.ID].State != "interrupted" {
		t.Fatal("interrupted job resumed automatically")
	}
	if _, err = recovered.undo(j.ID, "owner"); err == nil {
		t.Fatal("uncertain operation should require manual verification")
	}
	if _, err = os.Stat(filepath.Join(p, "a")); err != nil {
		t.Fatal("recovery changed source file")
	}
}

func TestPreflightDoesNotRequireSession(t *testing.T) {
	s, _ := setup(t)
	a := &App{store: s, mode: "tos"}
	r := httptest.NewRequest("OPTIONS", "http://nas:8181/api/info", nil)
	r.Header.Set("Origin", "http://nas:8181")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://nas:8181" {
		t.Fatal("preflight failed")
	}
}

func TestOriginFailureHasDistinctDiagnostic(t *testing.T) {
	s, _ := setup(t)
	a := &App{store: s, mode: "tos"}
	for _, method := range []string{"GET", "OPTIONS"} {
		r := httptest.NewRequest(method, "http://nas:8181/v2/proxy/batch-rename/api/info", nil)
		r.Header.Set("Origin", "http://foreign-origin")
		r.Header.Set("Cookie", "TMSESSNAME=s; X-Csrf-Token=c")
		r.Header.Set("X-Csrf-Token", "c")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		var body map[string]string
		json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != 403 || body["code"] != "ORIGIN_MISMATCH" {
			t.Fatalf("missing source diagnostic: %d %s", w.Code, w.Body)
		}
		if strings.Contains(body["error"], "联系管理员") {
			t.Fatal("source error mislabeled as account permission")
		}
	}
}

func TestLocalPreviewAcceptsCanonicalPlatformAPIRoute(t *testing.T) {
	s, _ := setup(t)
	a := &App{store: s, mode: "demo"}
	r := httptest.NewRequest("GET", "http://127.0.0.1:18946/v2/proxy/batch-rename/api/info", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("canonical proxy route failed locally: %d %s", w.Code, w.Body)
	}
}
