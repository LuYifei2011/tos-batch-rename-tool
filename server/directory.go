package main

import (
	"errors"
	"os"
)

// A directory is scoped to one authorized root and one request's credentials.
type directory interface {
	list() ([]Entry, error)
	check(Change) error
	rename(Change) (bool, error)
	Close() error
}

type nativeDirectory struct{ *os.File }

func (d nativeDirectory) list() ([]Entry, error) { return scan(d.File) }
func (d nativeDirectory) check(c Change) error {
	st, err := statAt(d.File, c.From)
	if err == nil && identity(&st) != c.Fingerprint {
		err = errors.New("源文件自预览后已变化")
	}
	return err
}
func (d nativeDirectory) rename(c Change) (bool, error) {
	if err := renameExclusive(d.File, c.From, c.To); err != nil {
		return false, err
	}
	return true, d.Sync()
}

type openDirectory func(root, path string) (directory, error)

func (s *Store) openNative(root, path string) (directory, error) {
	r := s.roots[root]
	if r == nil {
		return nil, errors.New("共享目录未授权")
	}
	d, err := openDir(r, path)
	if err != nil {
		return nil, err
	}
	return nativeDirectory{d}, nil
}
