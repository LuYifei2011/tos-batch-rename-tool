package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	tosapp "github.com/LuYifei2011/tos-app-kit/go"
)

type fakePlatform struct {
	mu                            sync.Mutex
	files                         map[string]platformEntry
	denied, expired, disconnected bool
	renames                       int
	pageSize                      int
	expireAfterRename             bool
}

func testCredentials(t *testing.T, session string) tosapp.Credentials {
	t.Helper()
	r := httptest.NewRequest("GET", "http://nas/api/info", nil)
	r.Header.Set("Cookie", "TMSESSNAME="+session+"; X-Csrf-Token=csrf")
	r.Header.Set("X-Csrf-Token", "csrf")
	credentials, err := tosapp.ReadCredentials(r)
	if err != nil {
		t.Fatal(err)
	}
	return credentials
}

func newFakePlatform(t *testing.T) (*fakePlatform, *platformSession) {
	t.Helper()
	f := &fakePlatform{files: map[string]platformEntry{}, pageSize: 1000}
	u := httptest.NewServer(f)
	t.Cleanup(u.Close)
	s := &platformSession{client: tosapp.NewPlatformClient(), base: u.URL, credentials: testCredentials(t, "alice")}
	t.Cleanup(s.client.CloseIdleConnections)
	return f, s
}
func (f *fakePlatform) put(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[name] = platformEntry{Name: name, Path: "/Volume1/Media/" + name, Type: "file", Size: "123", Modified: "1780962407", Owner: "alice"}
}
func (f *fakePlatform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ok := func(v any) { send(w, 200, map[string]any{"code": true, "is_login": true, "data": v}) }
	no := func() { send(w, 200, map[string]any{"code": false, "is_login": true}) }
	sess, _ := r.Cookie("TMSESSNAME")
	if sess == nil || (sess.Value != "alice" && sess.Value != "bob") || f.expired {
		send(w, 200, map[string]any{"code": false, "is_login": false})
		return
	}
	if r.Header.Get("X-Csrf-Token") != "csrf" {
		w.WriteHeader(403)
		return
	}
	switch strings.TrimPrefix(r.URL.Path, "/v2/fileManage/") {
	case "homeList":
		p := "/Volume1/Media"
		if sess.Value == "bob" {
			p = "/Volume1/Bob"
		}
		ok([]platformEntry{{Name: path.Base(p), Path: p, Type: "folder"}, {Name: "homes", Path: "/Volume1/homes", Type: "folder"}, {Name: "system", Path: "/Volume1/@apps", Type: "folder"}})
	case "list":
		if r.URL.Query().Get("path") != "/Volume1/Media" {
			no()
			return
		}
		names := []string{}
		for name := range f.files {
			names = append(names, name)
		}
		sort.Strings(names)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		start := (page - 1) * f.pageSize
		entries := []platformEntry{}
		for i := start; i < len(names) && i < start+f.pageSize; i++ {
			entries = append(entries, f.files[names[i]])
		}
		ok(map[string]any{"total": len(names), "total_page": (len(names) + f.pageSize - 1) / f.pageSize, "data": entries})
	case "folderInfoAll":
		entry, exists := f.files[path.Base(r.URL.Query().Get("path"))]
		if !exists || entry.Path != r.URL.Query().Get("path") {
			no()
			return
		}
		ok(entry)
	case "folderRename":
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		var req struct {
			Path string `json:"path"`
			Name string `json:"r_name"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		if f.disconnected {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		e, exists := f.files[path.Base(req.Path)]
		if _, occupied := f.files[req.Name]; f.denied || !exists || occupied || e.Path != req.Path {
			no()
			return
		}
		delete(f.files, e.Name)
		e.Name = req.Name
		e.Path = path.Join(path.Dir(e.Path), e.Name)
		f.files[e.Name] = e
		f.renames++
		if f.expireAfterRename {
			f.expired = true
		}
		ok(e)
	default:
		w.WriteHeader(404)
	}
}

func TestAutomaticTOSRootsAndUserIsolation(t *testing.T) {
	_, p := newFakePlatform(t)
	s, _ := setup(t)
	a := &App{store: s, mode: "tos", platform: p.client, platformBase: p.base}
	request := func(session, endpoint string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://nas:8181/api/"+endpoint, nil)
		r.Header.Set("Cookie", "TMSESSNAME="+session+"; X-Csrf-Token=csrf")
		r.Header.Set("X-Csrf-Token", "csrf")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	for _, user := range []string{"alice", "bob"} {
		w := request(user, "info")
		var info struct {
			Roots []RootConfig `json:"roots"`
		}
		json.Unmarshal(w.Body.Bytes(), &info)
		if w.Code != 200 || len(info.Roots) != 1 || info.Roots[0].Path != "" {
			t.Fatalf("unexpected discovery: %d %s", w.Code, w.Body)
		}
		if user == "bob" && request(user, "list?root="+rootID("/Volume1/Media")).Code < 400 {
			t.Fatal("other user's root accepted")
		}
	}
	if request("forged", "jobs").Code != 401 {
		t.Fatal("forged cookie accepted")
	}
}

func TestTOSPlanExecuteAndUndo(t *testing.T) {
	f, p := newFakePlatform(t)
	f.put("照片.jpg")
	f.put("b.txt")
	s, _ := setup(t)
	d, err := p.open(rootID("/Volume1/Media"), "")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := d.list()
	if err != nil {
		t.Fatal(err)
	}
	items := []Change{}
	for _, e := range entries {
		items = append(items, Change{From: e.Name, To: "new-" + e.Name, Fingerprint: e.Fingerprint})
	}
	plan, err := s.createPlanWith(p.open, rootID("/Volume1/Media"), "", "alice-hash", items)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.startWith(p.open, plan.ID, "alice-hash")
	if err != nil {
		t.Fatal(err)
	}
	if finish(t, s, j.ID).Done != 2 {
		t.Fatal("rename failed")
	}
	u, err := s.undoWith(p.open, j.ID, "alice-hash")
	if err != nil {
		t.Fatal(err)
	}
	uj, err := s.startWith(p.open, u.ID, "alice-hash")
	if err != nil {
		t.Fatal(err)
	}
	if finish(t, s, uj.ID).Done != 2 || f.renames != 4 {
		t.Fatal("undo failed")
	}
	files, _ := filepath.Glob(filepath.Join(s.data, "history", "*"))
	for _, name := range files {
		b, _ := os.ReadFile(name)
		if strings.Contains(string(b), "csrf") || strings.Contains(string(b), "TMSESSNAME") {
			t.Fatal("credentials persisted")
		}
	}
}

func TestTOSRevalidatesAtExecute(t *testing.T) {
	for _, mutation := range []string{"source", "target", "expired", "permission", "ambiguous"} {
		t.Run(mutation, func(t *testing.T) {
			f, p := newFakePlatform(t)
			f.put("a")
			s, _ := setup(t)
			e, _ := f.files["a"].entry()
			plan, err := s.createPlanWith(p.open, rootID("/Volume1/Media"), "", "owner", []Change{{From: "a", To: "b", Fingerprint: e.Fingerprint}})
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "source":
				e := f.files["a"]
				e.Size = "999"
				f.files["a"] = e
			case "target":
				f.put("b")
			case "expired":
				f.expired = true
			case "permission":
				f.denied = true
			case "ambiguous":
				f.disconnected = true
			}
			j, err := s.startWith(p.open, plan.ID, "owner")
			if mutation == "ambiguous" || mutation == "permission" {
				if err != nil {
					t.Fatal(err)
				}
				j = finish(t, s, j.ID)
				if j.Done != 0 || j.Failed != 1 {
					t.Fatalf("bad job: %+v", j)
				}
				if mutation == "ambiguous" && j.State != "interrupted" {
					t.Fatal("ambiguous response retried or accepted")
				}
			} else if err == nil {
				t.Fatal("stale plan executed")
			}
			if f.renames != 0 {
				t.Fatal("unexpected rename")
			}
		})
	}
}

func TestTOSPaginationAndPathBoundaries(t *testing.T) {
	f, p := newFakePlatform(t)
	f.pageSize = 2
	for i := 0; i < 5; i++ {
		f.put(fmt.Sprintf("f%d", i))
	}
	f.put("#recycle")
	d, _ := p.open(rootID("/Volume1/Media"), "")
	entries, err := d.list()
	if err != nil || len(entries) != 5 {
		t.Fatalf("pagination: %v %d", err, len(entries))
	}
	for _, rel := range []string{"../other", "/etc", "a/../../b", "a\\b", "#recycle", "@apps"} {
		if _, err := p.open(rootID("/Volume1/Media"), rel); err == nil {
			t.Fatalf("accepted %q", rel)
		}
	}
	for _, root := range []string{"/", "/etc", "/Volume1", "/Volume1/homes", "/Volume1/@apps/app", "/Volume1/Media/../Other", "/Volume1/Media/#recycle"} {
		if safePlatformPath(root) {
			t.Fatalf("accepted %q", root)
		}
	}
}

func TestTOSRejectRedirect(t *testing.T) {
	visits := 0
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visits++ }))
	defer external.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, external.URL, 302) }))
	defer upstream.Close()
	p := &platformSession{client: tosapp.NewPlatformClient(), base: upstream.URL, credentials: testCredentials(t, "secret")}
	defer p.client.CloseIdleConnections()
	if _, err := p.roots(t.Context()); err == nil || visits != 0 {
		t.Fatal("credentials followed a redirect")
	}
}

func TestTOSHTTPS5443Origin(t *testing.T) {
	s, _ := setup(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"code": true, "is_login": true, "data": []any{}})
	}))
	defer upstream.Close()
	client := tosapp.NewPlatformClient()
	defer client.CloseIdleConnections()
	a := &App{store: s, mode: "tos", platform: client, platformBase: upstream.URL}
	for _, tc := range []struct {
		name, host, origin, site string
		want                     bool
	}{
		{"proxy-omits-port", "nas", "https://nas:5443", "same-origin", true},
		{"proxy-retains-port", "nas:5443", "https://nas:5443", "same-origin", true},
		{"ip-host", "192.0.2.2", "https://192.0.2.2:5443", "same-origin", true},
		{"foreign-host", "nas", "https://other:5443", "same-origin", false},
		{"different-explicit-port", "nas:8181", "https://nas:5443", "same-origin", false},
		{"http-on-https-port", "nas", "http://nas:5443", "same-origin", false},
		{"unknown-stripped-port", "nas", "https://nas:9999", "same-origin", false},
		{"cross-site", "nas", "https://nas:5443", "cross-site", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://nas/api/info", nil)
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("Cookie", "TMSESSNAME=alice; X-Csrf-Token=csrf")
			r.Header.Set("X-Csrf-Token", "csrf")
			w := httptest.NewRecorder()
			a.ServeHTTP(w, r)
			want := http.StatusForbidden
			if tc.want {
				want = http.StatusOK
			}
			if w.Code != want {
				t.Fatalf("status = %v, want %v: %s", w.Code, want, w.Body)
			}
		})
	}
}

func TestTOSSessionExpiresDuringBatch(t *testing.T) {
	f, p := newFakePlatform(t)
	s, _ := setup(t)
	s.workers = 1
	items := []Change{}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("a%d", i)
		f.put(name)
		e, _ := f.files[name].entry()
		items = append(items, Change{From: name, To: "new-" + name, Fingerprint: e.Fingerprint})
	}
	plan, err := s.createPlanWith(p.open, rootID("/Volume1/Media"), "", "owner", items)
	if err != nil {
		t.Fatal(err)
	}
	f.expireAfterRename = true
	j, err := s.startWith(p.open, plan.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	j = finish(t, s, j.ID)
	if j.Done != 1 || j.Failed != 1 || j.State != "cancelled" || f.renames != 1 {
		t.Fatalf("expired session kept writing: %+v", j)
	}
}
