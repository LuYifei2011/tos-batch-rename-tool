package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxBatch = 50000

type Change struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Fingerprint string `json:"fingerprint"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
}
type Plan struct {
	ID      string   `json:"id"`
	Root    string   `json:"root"`
	Path    string   `json:"path"`
	Items   []Change `json:"items"`
	owner   string
	expires time.Time
	UndoOf  string `json:"undoOf,omitempty"`
}
type Job struct {
	ID       string   `json:"id"`
	Root     string   `json:"root"`
	Path     string   `json:"path"`
	State    string   `json:"state"`
	Total    int      `json:"total"`
	Done     int      `json:"done"`
	Failed   int      `json:"failed"`
	Started  string   `json:"started"`
	Finished string   `json:"finished,omitempty"`
	Items    []Change `json:"items,omitempty"`
	UndoOf   string   `json:"undoOf,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	stop     bool
}
type Store struct {
	mu      sync.Mutex
	plans   map[string]*Plan
	jobs    map[string]*Job
	active  bool
	data    string
	roots   map[string]*FileRoot
	workers int
}

func uid() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func newStore(data string, roots map[string]*FileRoot, workers int) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(data, "history"), 0700); err != nil {
		return nil, err
	}
	s := &Store{plans: map[string]*Plan{}, jobs: map[string]*Job{}, data: data, roots: roots, workers: workers}
	files, _ := filepath.Glob(filepath.Join(data, "history", "*.json"))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var j Job
		if json.Unmarshal(b, &j) != nil {
			return nil, fmt.Errorf("执行记录损坏: %s", p)
		}
		if raw, e := hex.DecodeString(j.ID); e != nil || len(raw) != 16 || filepath.Base(p) != j.ID+".json" {
			return nil, errors.New("执行记录 ID 无效")
		}
		if j.State == "running" {
			j.State = "interrupted"
			latest := map[string]Change{}
			f, e := os.Open(strings.TrimSuffix(p, ".json") + ".jsonl")
			if e == nil {
				scanner := bufio.NewScanner(f)
				scanner.Buffer(make([]byte, 4096), 8192)
				for scanner.Scan() {
					var c Change
					if json.Unmarshal(scanner.Bytes(), &c) == nil {
						latest[c.From] = c
					}
				}
				f.Close()
			}
			j.Done = 0
			j.Failed = 0
			for i, c := range j.Items {
				if result, ok := latest[c.From]; ok {
					j.Items[i] = result
				}
				if j.Items[i].State == "done" {
					j.Done++
				} else if j.Items[i].State == "failed" {
					j.Failed++
				}
			}
			if err := s.save(&j); err != nil {
				return nil, err
			}
		}
		j.Items = nil
		s.jobs[j.ID] = &j
	}
	return s, nil
}
func (s *Store) createPlan(root, path, owner string, items []Change) (*Plan, error) {
	return s.createPlanWith(s.openNative, root, path, owner, items)
}
func (s *Store) createPlanWith(open openDirectory, root, path, owner string, items []Change) (*Plan, error) {
	if len(items) == 0 || len(items) > maxBatch {
		return nil, errors.New("每批需包含 1 至 50,000 项")
	}
	d, err := open(root, path)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	entries, err := d.list()
	if err != nil {
		return nil, err
	}
	copyItems, err := validateChanges(entries, items)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.plans {
		if time.Now().After(p.expires) {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 4 {
		return nil, errors.New("待确认预览过多，请稍后重试")
	}
	p := &Plan{ID: uid(), Root: root, Path: path, Items: copyItems, owner: owner, expires: time.Now().Add(5 * time.Minute)}
	s.plans[p.ID] = p
	return p, nil
}
func validateChanges(entries []Entry, items []Change) ([]Change, error) {
	existing := map[string]Entry{}
	fold := map[string]bool{}
	for _, e := range entries {
		existing[e.Name] = e
		fold[strings.ToLower(e.Name)] = true
	}
	sources := map[string]bool{}
	targets := map[string]bool{}
	copyItems := make([]Change, len(items))
	for i, c := range items {
		e, ok := existing[c.From]
		if !ok || e.Kind == "symlink" || e.Fingerprint != c.Fingerprint {
			return nil, fmt.Errorf("源文件已变化，请刷新: %s", c.From)
		}
		if strings.ContainsAny(c.From, "/\\\x00") || !validName(c.To) || c.From == c.To {
			return nil, fmt.Errorf("文件名无效: %s", c.To)
		}
		key := strings.ToLower(c.To)
		if sources[c.From] || targets[key] || fold[key] {
			return nil, fmt.Errorf("目标名称冲突: %s", c.To)
		}
		sources[c.From] = true
		targets[key] = true
		c.State = "pending"
		c.Error = ""
		copyItems[i] = c
	}
	return copyItems, nil
}
func (s *Store) save(j *Job) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	p := filepath.Join(s.data, "history", j.ID+".json")
	f, err := os.OpenFile(p+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return err
	}
	if err = os.Rename(p+".part", p); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(p))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (s *Store) start(id, owner string) (*Job, error) {
	return s.startWith(s.openNative, id, owner)
}
func (s *Store) startWith(open openDirectory, id, owner string) (*Job, error) {
	s.mu.Lock()
	p := s.plans[id]
	if p == nil || p.owner != owner || time.Now().After(p.expires) {
		s.mu.Unlock()
		return nil, errors.New("预览已过期，请重新校验")
	}
	if s.active {
		s.mu.Unlock()
		return nil, errors.New("已有任务正在执行")
	}
	s.active = true
	s.mu.Unlock()
	committed := false
	defer func() {
		if !committed {
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
		}
	}()
	d, err := open(p.Root, p.Path)
	if err != nil {
		return nil, err
	}
	// Platform calls have no compare-and-rename primitive; repeat the full conflict check at confirmation.
	if _, ok := d.(*platformDirectory); ok {
		entries, e := d.list()
		if e == nil {
			_, e = validateChanges(entries, p.Items)
		}
		if e != nil {
			d.Close()
			return nil, e
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plans[id] != p || time.Now().After(p.expires) {
		d.Close()
		return nil, errors.New("预览已过期，请重新校验")
	}
	if err = s.makeHistoryRoom(); err != nil {
		d.Close()
		return nil, err
	}
	j := &Job{ID: uid(), Root: p.Root, Path: p.Path, State: "running", Total: len(p.Items), Started: time.Now().UTC().Format(time.RFC3339), Items: append([]Change(nil), p.Items...), Owner: owner, UndoOf: p.UndoOf}
	if err = s.save(j); err != nil {
		d.Close()
		return nil, err
	}
	delete(s.plans, id)
	s.jobs[j.ID] = j
	if pd, ok := d.(*platformDirectory); ok {
		pd.ctx = context.Background()
	}
	committed = true
	go s.run(j, d)
	return j, nil
}

// Keep the most recent 32 batches. Uncertain operations must remain available for review.
func (s *Store) makeHistoryRoom() error {
	if len(s.jobs) < 32 {
		return nil
	}
	finished := []*Job{}
	for _, j := range s.jobs {
		if j.State == "done" || j.State == "partial" || j.State == "cancelled" {
			finished = append(finished, j)
		}
	}
	sort.Slice(finished, func(i, j int) bool { return finished[i].Started < finished[j].Started })
	for _, j := range finished {
		if len(s.jobs) < 32 {
			return nil
		}
		for _, ext := range []string{".jsonl", ".json.part", ".json"} {
			if err := os.Remove(filepath.Join(s.data, "history", j.ID+ext)); err != nil && !os.IsNotExist(err) {
				return errors.New("无法轮转执行记录")
			}
		}
		delete(s.jobs, j.ID)
	}
	if len(s.jobs) >= 32 {
		return errors.New("待核对的中断记录已达 32 批，请先核对文件状态")
	}
	return nil
}
func (s *Store) run(j *Job, d directory) {
	defer d.Close()
	log, err := os.OpenFile(filepath.Join(s.data, "history", j.ID+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		s.mu.Lock()
		j.State = "interrupted"
		s.active = false
		s.save(j)
		s.mu.Unlock()
		return
	}
	defer log.Close()
	var logMu sync.Mutex
	record := func(c Change) error {
		logMu.Lock()
		defer logMu.Unlock()
		b, e := json.Marshal(c)
		if e != nil {
			return e
		}
		if _, e = log.Write(append(b, '\n')); e != nil {
			return e
		}
		return log.Sync()
	}
	queue := make(chan int)
	var wg sync.WaitGroup
	for n := 0; n < s.workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				s.mu.Lock()
				stop := j.stop
				c := j.Items[i]
				s.mu.Unlock()
				if stop {
					continue
				}
				e := d.check(c)
				if e == nil {
					c.State = "intent"
					e = record(c)
				}
				moved := false
				if e == nil {
					moved, e = d.rename(c)
				}
				if e != nil {
					c.State = "failed"
					if moved {
						c.State = "uncertain"
					}
					c.Error = e.Error()
				} else {
					c.State = "done"
				}
				if re := record(c); re != nil {
					c.State = "uncertain"
					c.Error = "记录写入失败，请核对文件实际名称"
					s.mu.Lock()
					j.stop = true
					s.mu.Unlock()
				}
				var platformErr *platformError
				if c.State == "uncertain" || errors.As(e, &platformErr) {
					s.mu.Lock()
					j.stop = true
					s.mu.Unlock()
				}
				s.mu.Lock()
				j.Items[i] = c
				if c.State == "done" {
					j.Done++
				} else {
					j.Failed++
				}
				s.mu.Unlock()
			}
		}()
	}
	for i := range j.Items {
		queue <- i
	}
	close(queue)
	wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	j.State = "done"
	if j.stop {
		j.State = "cancelled"
	} else if j.Failed > 0 {
		j.State = "partial"
	}
	for _, c := range j.Items {
		if c.State == "uncertain" {
			j.State = "interrupted"
		}
	}
	j.Finished = time.Now().UTC().Format(time.RFC3339)
	if s.save(j) != nil {
		j.State = "interrupted"
	} else {
		j.Items = nil
	}
	s.active = false
}
func (s *Store) undo(id, owner string) (*Plan, error) {
	return s.undoWith(s.openNative, id, owner)
}
func (s *Store) undoWith(open openDirectory, id, owner string) (*Plan, error) {
	s.mu.Lock()
	j := s.jobs[id]
	if j == nil || j.Owner != owner || j.State == "running" || j.State == "interrupted" || j.UndoOf != "" {
		s.mu.Unlock()
		return nil, errors.New("此记录不能撤销")
	}
	items := []Change{}
	var record Job
	b, readErr := os.ReadFile(filepath.Join(s.data, "history", j.ID+".json"))
	if readErr != nil || json.Unmarshal(b, &record) != nil {
		s.mu.Unlock()
		return nil, errors.New("执行记录无法读取")
	}
	for _, c := range record.Items {
		if c.State == "done" {
			items = append(items, Change{From: c.To, To: c.From, Fingerprint: c.Fingerprint})
		}
	}
	root, path := j.Root, j.Path
	s.mu.Unlock()
	p, err := s.createPlanWith(open, root, path, owner, items)
	if err == nil {
		s.mu.Lock()
		p.UndoOf = id
		s.mu.Unlock()
	}
	return p, err
}
