package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	tosapp "github.com/LuYifei2011/tos-app-kit/go"
)

type App struct {
	store        *Store
	roots        []RootConfig
	mode, web    string
	platform     *http.Client
	platformBase string
}

var appVersion = "dev"

func send(w http.ResponseWriter, status int, v any) {
	tosapp.WriteJSON(w, status, v)
}
func fail(w http.ResponseWriter, status int, err error) {
	var pe *platformError
	if errors.As(err, &pe) {
		status = pe.status
	}
	send(w, status, map[string]string{"error": err.Error()})
}
func rejectOrigin(w http.ResponseWriter) {
	send(w, http.StatusForbidden, map[string]string{"code": "ORIGIN_MISMATCH", "error": "连接来源校验失败（ORIGIN_MISMATCH），请从 TOS 或 TNAS Mobile 重新打开应用。"})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	err := tosapp.DecodeJSON(w, r, v, 48<<20)
	if errors.Is(err, tosapp.ErrTrailingJSON) {
		return errors.New("请求包含多余内容")
	}
	if err != nil {
		return errors.New("请求体无效或超过大小限制")
	}
	return nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Batch-Rename-Version", appVersion)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	p := tosapp.StripAppPrefix(r.URL.Path, "batch-rename")
	if !strings.HasPrefix(p, "/api/") {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; worker-src 'self'; frame-ancestors 'self'; object-src 'none'; base-uri 'self'")
		http.FileServer(http.Dir(a.web)).ServeHTTP(w, r)
		return
	}
	owner := "local-demo"
	open := a.store.openNative
	roots := a.roots
	if r.Method == "OPTIONS" {
		if a.mode != "tos" {
			rejectOrigin(w)
			return
		}
		if err := tosapp.WritePreflight(w, r, []string{"GET", "POST", "OPTIONS"}); err != nil {
			rejectOrigin(w)
		}
		return
	}
	if a.mode == "tos" {
		base := a.platformBase
		if base == "" {
			base = "http://127.0.0.1:8181"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
		defer cancel()
		var ps *platformSession
		credentials, err := tosapp.Authenticate(r.WithContext(ctx), func(ctx context.Context, credentials tosapp.Credentials) error {
			ps = &platformSession{client: a.platform, base: base, credentials: credentials, ctx: ctx}
			var err error
			roots, err = ps.roots(ctx)
			return err
		})
		if err != nil {
			switch {
			case errors.Is(err, tosapp.ErrInvalidCredentials):
				send(w, 401, map[string]string{"code": "AUTH_HEADERS_INVALID", "error": "TOS 会话或 CSRF 校验失败"})
			case errors.Is(err, tosapp.ErrOriginMismatch):
				rejectOrigin(w)
			default:
				fail(w, 503, err)
			}
			return
		}
		owner = credentials.SessionHash()
		open = ps.open
	} else {
		if o := r.Header.Get("Origin"); o != "" {
			u, e := url.Parse(o)
			if e != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
				fail(w, 403, errors.New("仅允许本地访问"))
				return
			}
		}
		host, _, _ := net.SplitHostPort(r.Host)
		if host != "127.0.0.1" && host != "localhost" {
			fail(w, 403, errors.New("主机名未授权"))
			return
		}
	}
	switch {
	case p == "/api/info" && r.Method == "GET":
		publicRoots := []RootConfig{}
		for _, rt := range roots {
			publicRoots = append(publicRoots, RootConfig{ID: rt.ID, Name: rt.Name})
		}
		send(w, 200, map[string]any{"mode": a.mode, "version": appVersion, "roots": publicRoots, "maxBatch": maxBatch, "workers": a.store.workers})
	case p == "/api/list" && r.Method == "GET":
		d, e := open(r.URL.Query().Get("root"), r.URL.Query().Get("path"))
		if e != nil {
			fail(w, 400, e)
			return
		}
		defer d.Close()
		entries, e := d.list()
		if e != nil {
			fail(w, 400, e)
			return
		}
		send(w, 200, map[string]any{"entries": entries})
	case p == "/api/plans" && r.Method == "POST":
		var req struct {
			Root  string   `json:"root"`
			Path  string   `json:"path"`
			Items []Change `json:"items"`
		}
		if e := decode(w, r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		plan, e := a.store.createPlanWith(open, req.Root, req.Path, owner, req.Items)
		if e != nil {
			fail(w, 409, e)
			return
		}
		send(w, 200, map[string]any{"id": plan.ID, "count": len(plan.Items), "expiresIn": 300})
	case p == "/api/execute" && r.Method == "POST":
		var req struct {
			Plan string `json:"plan"`
		}
		if e := decode(w, r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		j, e := a.store.startWith(open, req.Plan, owner)
		if e != nil {
			fail(w, 409, e)
			return
		}
		send(w, 202, map[string]string{"id": j.ID})
	case p == "/api/undo" && r.Method == "POST":
		var req struct {
			ID string `json:"id"`
		}
		if e := decode(w, r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		plan, e := a.store.undoWith(open, req.ID, owner)
		if e != nil {
			fail(w, 409, e)
			return
		}
		send(w, 200, map[string]any{"id": plan.ID, "count": len(plan.Items), "expiresIn": 300})
	case p == "/api/cancel" && r.Method == "POST":
		var req struct {
			ID string `json:"id"`
		}
		if e := decode(w, r, &req); e != nil {
			fail(w, 400, e)
			return
		}
		a.store.mu.Lock()
		j := a.store.jobs[req.ID]
		if j == nil || j.Owner != owner {
			a.store.mu.Unlock()
			fail(w, 404, errors.New("记录不存在"))
			return
		}
		j.stop = true
		a.store.mu.Unlock()
		send(w, 200, map[string]bool{"ok": true})
	case p == "/api/jobs" && r.Method == "GET":
		a.store.mu.Lock()
		defer a.store.mu.Unlock()
		jobs := []Job{}
		for _, j := range a.store.jobs {
			if j.Owner == owner {
				copy := *j
				copy.Items = nil
				copy.Owner = ""
				jobs = append(jobs, copy)
			}
		}
		sort.Slice(jobs, func(i, j int) bool { return jobs[i].Started > jobs[j].Started })
		send(w, 200, jobs)
	case strings.HasPrefix(p, "/api/jobs/") && r.Method == "GET":
		a.store.mu.Lock()
		defer a.store.mu.Unlock()
		j := a.store.jobs[strings.TrimPrefix(p, "/api/jobs/")]
		if j == nil || j.Owner != owner {
			fail(w, 404, errors.New("记录不存在"))
			return
		}
		copy := *j
		if r.URL.Query().Get("summary") == "1" {
			copy.Items = nil
		} else if copy.Items == nil {
			b, e := os.ReadFile(filepath.Join(a.store.data, "history", j.ID+".json"))
			if e != nil || json.Unmarshal(b, &copy) != nil {
				fail(w, 500, errors.New("执行记录无法读取"))
				return
			}
		}
		copy.Owner = ""
		send(w, 200, copy)
	default:
		fail(w, 404, errors.New("接口不存在"))
	}
}
func demoData(data string) ([]RootConfig, error) {
	p := filepath.Join(data, "demo")
	if err := os.MkdirAll(p, 0700); err != nil {
		return nil, err
	}
	marker := filepath.Join(data, "demo-initialized")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		for _, dir := range []string{"照片整理", "项目文档", "视频素材"} {
			if e := os.MkdirAll(filepath.Join(p, dir), 0700); e != nil {
				return nil, e
			}
		}
		for i := 1; i <= 28; i++ {
			name := fmt.Sprintf("IMG_202611_%04d.jpg", i)
			if i > 18 {
				name = fmt.Sprintf("DSC_%04d.RAW", i)
			}
			if e := os.WriteFile(filepath.Join(p, "照片整理", name), []byte("Batch Rename demonstration fixture. Not an image.\n"), 0600); e != nil {
				return nil, e
			}
		}
		for _, name := range []string{"会议记录 终稿.txt", "会议记录 草稿.txt", "项目说明.txt"} {
			os.WriteFile(filepath.Join(p, "项目文档", name), []byte("演示文件\n"), 0600)
		}
		os.WriteFile(marker, []byte("1"), 0600)
	}
	return []RootConfig{{ID: "demo", Name: "演示共享目录", Path: p}}, nil
}
func main() {
	prepare := flag.Bool("prepare-data", false, "prepare the fixed TOS data directory and exit")
	mode := flag.String("mode", "demo", "demo or tos")
	data := flag.String("data", ".local/data", "runtime data directory")
	web := flag.String("web", "dist", "frontend directory")
	listen := flag.String("listen", "127.0.0.1:18946", "local demo listener")
	workers := flag.Int("workers", 4, "concurrent rename operations (1-8)")
	flag.Parse()
	if *prepare {
		if err := tosapp.PrepareAppData("batch-rename"); err != nil {
			log.Fatalf("[ERROR] [initialize] %v", err)
		}
		return
	}
	if *mode != "demo" && *mode != "tos" {
		log.Fatal("mode must be demo or tos")
	}
	if *workers < 1 || *workers > 8 {
		log.Fatal("workers must be 1-8")
	}
	if *mode == "tos" {
		if err := tosapp.RequireNonRoot(); err != nil {
			log.Fatal(err)
		}
	}
	absData, err := filepath.Abs(*data)
	if err != nil {
		log.Fatal(err)
	}
	if *mode == "tos" {
		if err := tosapp.CheckDataDirectory(absData); err != nil {
			log.Fatalf("[ERROR] [startup] %v", err)
		}
	}
	var roots []RootConfig
	if *mode == "demo" {
		roots, err = demoData(absData)
	}
	if err != nil {
		log.Fatal(err)
	}
	opened := map[string]*FileRoot{}
	for _, r := range roots {
		if r.ID == "" || opened[r.ID] != nil {
			log.Fatal("invalid root ID")
		}
		p, e := rootPath(r.Path)
		if e != nil {
			log.Fatal(e)
		}
		f, e := os.Open(p)
		if e != nil {
			log.Fatal(e)
		}
		st, e := f.Stat()
		if e != nil || !st.IsDir() {
			log.Fatal("shared root must be directory")
		}
		opened[r.ID] = &FileRoot{r, f}
		defer f.Close()
	}
	store, err := newStore(absData, opened, *workers)
	if err != nil {
		log.Fatalf("[ERROR] [history] cannot open execution records; preserve data/history and inspect its permissions and JSON files: %v", err)
	}
	platform := tosapp.NewPlatformClient()
	defer platform.CloseIdleConnections()
	app := &App{store: store, roots: roots, mode: *mode, web: *web, platform: platform}
	var ln net.Listener
	if *mode == "tos" {
		ln, err = tosapp.SystemdListener()
	} else {
		ln, err = tosapp.ListenLoopback(*listen)
	}
	if err != nil {
		log.Fatal(err)
	}
	srv := tosapp.NewHTTPServer(app)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	log.Printf("[INFO] [startup] batch-rename %s ready (%s), %d authorized roots, %d workers", appVersion, *mode, len(roots), *workers)
	serveErr := tosapp.Serve(ctx, srv, ln, tosapp.ServeOptions{BeforeShutdown: func() {
		store.mu.Lock()
		for _, j := range store.jobs {
			j.stop = true
		}
		store.mu.Unlock()
	}})
	for {
		store.mu.Lock()
		active := store.active
		store.mu.Unlock()
		if !active {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if serveErr != nil {
		log.Fatal(serveErr)
	}
}
