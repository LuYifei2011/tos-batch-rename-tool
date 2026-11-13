package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
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
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
		return false
	}
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	// TOS nginx uses $host in some builds, which omits the public port.
	proxyHost := &url.URL{Host: r.Host}
	platformPort := u.Port() == "8181" || u.Port() == "8443" || (u.Scheme == "https" && u.Port() == "5443")
	return proxyHost.Port() == "" && strings.EqualFold(u.Hostname(), proxyHost.Hostname()) && platformPort
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 48<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("请求体无效或超过大小限制")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("请求包含多余内容")
	}
	return nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Batch-Rename-Version", appVersion)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	p := r.URL.Path
	p = strings.TrimPrefix(p, "/v2/proxy/batch-rename")
	p = strings.TrimPrefix(p, "/batch-rename")
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
		if a.mode != "tos" || r.Header.Get("Origin") == "" || !sameOrigin(r) {
			rejectOrigin(w)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Csrf-Token, Cookie")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")
		w.WriteHeader(204)
		return
	}
	if a.mode == "tos" {
		session, err := r.Cookie("TMSESSNAME")
		csrf, e2 := r.Cookie("X-Csrf-Token")
		token := r.Header.Get("X-Csrf-Token")
		if err != nil || e2 != nil || session.Value == "" || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(csrf.Value)) != 1 {
			send(w, 401, map[string]string{"code": "AUTH_HEADERS_INVALID", "error": "TOS 会话或 CSRF 校验失败"})
			return
		}
		hash := sha256.Sum256([]byte(session.Value))
		owner = hex.EncodeToString(hash[:])
		if !sameOrigin(r) {
			rejectOrigin(w)
			return
		}
		base := a.platformBase
		if base == "" {
			base = "http://127.0.0.1:8181"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
		defer cancel()
		ps := &platformSession{client: a.platform, base: base, cookie: session.String() + "; " + csrf.String(), csrf: token, ctx: ctx}
		var e error
		roots, e = ps.roots(ctx)
		if e != nil {
			fail(w, 503, e)
			return
		}
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
		if err := prepareTOSData(); err != nil {
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
	if *mode == "tos" && os.Geteuid() == 0 {
		log.Fatal("TOS requires a non-root service user")
	}
	absData, err := filepath.Abs(*data)
	if err != nil {
		log.Fatal(err)
	}
	if *mode == "tos" {
		if err := checkTOSData(absData); err != nil {
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
	app := &App{store: store, roots: roots, mode: *mode, web: *web, platform: platformClient()}
	var ln net.Listener
	if *mode == "tos" {
		if os.Getenv("LISTEN_FDS") != "1" || os.Getenv("LISTEN_PID") != fmt.Sprint(os.Getpid()) {
			log.Fatal("TOS mode requires systemd socket activation")
		}
		f := os.NewFile(3, "systemd-socket")
		ln, err = net.FileListener(f)
		f.Close()
	} else {
		host, _, e := net.SplitHostPort(*listen)
		if e != nil || host != "127.0.0.1" {
			log.Fatal("demo binds to 127.0.0.1 only")
		}
		ln, err = net.Listen("tcp", *listen)
	}
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		store.mu.Lock()
		for _, j := range store.jobs {
			j.stop = true
		}
		store.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	log.Printf("[INFO] [startup] batch-rename %s ready (%s), %d authorized roots, %d workers", appVersion, *mode, len(roots), *workers)
	if e := srv.Serve(ln); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
	for {
		store.mu.Lock()
		active := store.active
		store.mu.Unlock()
		if !active {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}
