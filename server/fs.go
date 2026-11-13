package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxEntries = 100000

type RootConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}
type FileRoot struct {
	RootConfig
	file *os.File
}
type Entry struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	Modified    string `json:"modified"`
	Fingerprint string `json:"fingerprint"`
}

var reserved = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)`)

func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.EqualFold(s, "#recycle") && len(s) <= 255 && utf8.ValidString(s) && !strings.ContainsAny(s, "/\\\x00<>:\"|?*") && !strings.ContainsFunc(s, func(r rune) bool { return r < 32 }) && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, " ") && !reserved.MatchString(s)
}
func identity(s *unix.Stat_t) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d:%d:%d", s.Dev, s.Ino, s.Size, s.Mode, mtime(s).UnixNano(), s.Nlink)))
	return hex.EncodeToString(h[:16])
}
func openDir(root *FileRoot, path string) (*os.File, error) {
	if strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.ContainsRune(path, 0) {
		return nil, errors.New("目录路径无效")
	}
	fd, err := unix.Openat(int(root.file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			unix.Close(fd)
			return nil, errors.New("不允许访问父级目录")
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return nil, errors.New("目录不可访问，符号链接目录不会被跟随")
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), path), nil
}
func statAt(d *os.File, name string) (unix.Stat_t, error) {
	var s unix.Stat_t
	err := unix.Fstatat(int(d.Fd()), name, &s, unix.AT_SYMLINK_NOFOLLOW)
	return s, err
}
func scan(d *os.File) ([]Entry, error) {
	names, err := d.Readdirnames(maxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > maxEntries {
		return nil, errors.New("单个目录超过 100,000 项，请拆分目录后处理")
	}
	out := make([]Entry, 0, len(names))
	for _, name := range names {
		if strings.EqualFold(name, "#recycle") {
			continue
		}
		s, e := statAt(d, name)
		if e != nil {
			return nil, fmt.Errorf("目录扫描期间条目发生变化，请刷新: %s", name)
		}
		kind := "file"
		switch s.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			kind = "dir"
		case unix.S_IFLNK:
			kind = "symlink"
		case unix.S_IFREG:
		default:
			continue
		}
		out = append(out, Entry{name, kind, s.Size, mtime(&s).UTC().Format(time.RFC3339), identity(&s)})
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Kind == "dir") != (out[j].Kind == "dir") {
			return out[i].Kind == "dir"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func rootPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}
