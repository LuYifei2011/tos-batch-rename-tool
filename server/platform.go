package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	tosapp "github.com/LuYifei2011/tos-app-kit/go"
)

type platformError struct {
	status    int
	text      string
	ambiguous bool
}

func (e *platformError) Error() string { return e.text }

// Credentials live only in an in-flight request or running job, never in plans or journals.
type platformSession struct {
	client      *http.Client
	credentials tosapp.Credentials
	base        string
	ctx         context.Context
}

func (s *platformSession) context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *platformSession) call(ctx context.Context, method, endpoint string, body any, out any) error {
	var b io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		b = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+"/v2/fileManage/"+endpoint, b)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if err := s.credentials.Apply(req.Header); err != nil {
		return err
	}
	res, err := s.client.Do(req)
	if err != nil {
		return &platformError{503, "TOS 文件服务暂时不可用", method != "GET"}
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return &platformError{401, "TOS 会话已失效", false}
	}
	if res.StatusCode == 403 {
		return &platformError{409, "TOS 拒绝访问此目录或文件", false}
	}
	if res.StatusCode != 200 {
		return &platformError{502, "TOS 文件服务请求失败", method != "GET"}
	}
	var payload struct {
		Login *bool           `json:"is_login"`
		Code  bool            `json:"code"`
		Data  json.RawMessage `json:"data"`
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20+1))
	if err != nil || len(data) > 8<<20 || json.Unmarshal(data, &payload) != nil {
		return &platformError{502, "TOS 文件服务返回格式无效", method != "GET"}
	}
	if payload.Login != nil && !*payload.Login {
		return &platformError{401, "TOS 会话已失效", false}
	}
	if payload.Login == nil {
		return &platformError{502, "TOS 文件服务未返回登录状态", method != "GET"}
	}
	if !payload.Code {
		return &platformError{409, "TOS 拒绝操作，请检查目录权限或名称冲突", false}
	}
	if out != nil && json.Unmarshal(payload.Data, out) != nil {
		return &platformError{502, "TOS 文件服务数据格式无效", method != "GET"}
	}
	return nil
}

type platformEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"f_type"`
	RealType string      `json:"f_real_type"`
	Size     json.Number `json:"size"`
	Modified json.Number `json:"m_time_stamp"`
	Owner    string      `json:"owner"`
	ReadOnly bool        `json:"is_only_read"`
}

func (p platformEntry) entry() (Entry, error) {
	if p.Name == "" || strings.ContainsAny(p.Name, "/\\\x00") || p.Name == "." || p.Name == ".." {
		return Entry{}, errors.New("TOS 返回的文件名无效")
	}
	kind := "file"
	if p.Type == "folder" {
		kind = "dir"
	}
	if strings.Contains(strings.ToLower(p.Type+" "+p.RealType), "link") {
		kind = "symlink"
	}
	size, err := strconv.ParseInt(p.Size.String(), 10, 64)
	if err != nil || size < 0 {
		return Entry{}, errors.New("TOS 返回的文件大小无效")
	}
	mt, err := strconv.ParseInt(p.Modified.String(), 10, 64)
	if err != nil {
		return Entry{}, errors.New("TOS 返回的修改时间无效")
	}
	// The platform does not expose inode identity. Names and ctime change on rename.
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s", p.Type, p.Size, p.Modified, p.Owner)))
	return Entry{p.Name, kind, size, time.Unix(mt, 0).UTC().Format(time.RFC3339), "tos-" + hex.EncodeToString(h[:16])}, nil
}

func rootID(p string) string {
	h := sha256.Sum256([]byte(p))
	return "tos-" + hex.EncodeToString(h[:16])
}
func safePlatformPath(p string) bool {
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.ContainsAny(p, "\\\x00") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "Volume") || strings.TrimPrefix(parts[0], "Volume") == "" {
		return false
	}
	if _, err := strconv.Atoi(strings.TrimPrefix(parts[0], "Volume")); err != nil {
		return false
	}
	for _, part := range parts[1:] {
		if part == "" || strings.HasPrefix(part, "@") || strings.EqualFold(part, "#recycle") {
			return false
		}
	}
	return !(len(parts) == 2 && strings.EqualFold(parts[1], "homes"))
}

func (s *platformSession) roots(ctx context.Context) ([]RootConfig, error) {
	var entries []platformEntry
	if err := s.call(ctx, "GET", "homeList", nil, &entries); err != nil {
		return nil, err
	}
	result := []RootConfig{}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Type != "folder" || !safePlatformPath(e.Path) || seen[e.Path] {
			continue
		}
		seen[e.Path] = true
		name := e.Name
		if name == "" {
			name = path.Base(e.Path)
		}
		result = append(result, RootConfig{ID: rootID(e.Path), Name: name, Path: e.Path})
	}
	return result, nil
}

func (s *platformSession) open(root, rel string) (directory, error) {
	if strings.HasPrefix(rel, "/") || strings.ContainsAny(rel, "\\\x00") {
		return nil, errors.New("目录路径无效")
	}
	for _, c := range strings.Split(rel, "/") {
		if c == ".." || strings.EqualFold(c, "#recycle") || strings.HasPrefix(c, "@") {
			return nil, errors.New("目录路径无效")
		}
	}
	roots, err := s.roots(s.context())
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if r.ID == root {
			p := path.Join(r.Path, rel)
			if !safePlatformPath(p) {
				return nil, errors.New("目录路径无效")
			}
			return &platformDirectory{session: s, path: p, ctx: s.context()}, nil
		}
	}
	return nil, errors.New("当前 TOS 用户无权访问此目录")
}

type platformDirectory struct {
	session *platformSession
	path    string
	ctx     context.Context
}

func (d *platformDirectory) Close() error { return nil }
func (d *platformDirectory) list() ([]Entry, error) {
	result := []Entry{}
	seen := map[string]bool{}
	total := -1
	for page := 1; page <= maxEntries/1000+1; page++ {
		var data struct {
			Total   int             `json:"total"`
			Pages   int             `json:"total_page"`
			Entries []platformEntry `json:"data"`
		}
		q := url.Values{"path": {d.path}, "rule": {"name"}, "limit": {"1000"}, "order": {"false"}, "page": {strconv.Itoa(page)}}
		if err := d.session.call(d.ctx, "GET", "list?"+q.Encode(), nil, &data); err != nil {
			return nil, err
		}
		if data.Total > maxEntries {
			return nil, errors.New("单个目录超过 100,000 项，请拆分目录后处理")
		}
		if data.Total < 0 || len(seen)+len(data.Entries) > data.Total {
			return nil, errors.New("TOS 目录分页数据无效")
		}
		if total < 0 {
			total = data.Total
		} else if total != data.Total {
			return nil, errors.New("目录扫描期间发生变化，请刷新")
		}
		for _, item := range data.Entries {
			if seen[item.Name] {
				return nil, errors.New("目录扫描期间发生变化，请刷新")
			}
			seen[item.Name] = true
			if strings.EqualFold(item.Name, "#recycle") {
				continue
			}
			if item.Path != path.Join(d.path, item.Name) {
				return nil, errors.New("TOS 返回的条目不在当前目录中")
			}
			e, err := item.entry()
			if err != nil {
				return nil, err
			}
			result = append(result, e)
		}
		if len(seen) == total {
			break
		}
		if len(data.Entries) == 0 || page == maxEntries/1000+1 {
			return nil, errors.New("TOS 目录分页不完整，请刷新")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if (result[i].Kind == "dir") != (result[j].Kind == "dir") {
			return result[i].Kind == "dir"
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
func (d *platformDirectory) check(c Change) error {
	var item platformEntry
	if err := d.session.call(d.ctx, "GET", "folderInfoAll?path="+url.QueryEscape(path.Join(d.path, c.From)), nil, &item); err != nil {
		return err
	}
	e, err := item.entry()
	if err != nil {
		return err
	}
	if item.Path != path.Join(d.path, c.From) || item.ReadOnly || e.Kind == "symlink" || e.Fingerprint != c.Fingerprint {
		return errors.New("源文件已变化或不可写，请刷新")
	}
	return nil
}
func (d *platformDirectory) rename(c Change) (bool, error) {
	err := d.session.call(d.ctx, "POST", "folderRename", map[string]string{"path": path.Join(d.path, c.From), "r_name": c.To}, nil)
	var pe *platformError
	if errors.As(err, &pe) && pe.ambiguous {
		return true, err
	}
	return err == nil, err
}
