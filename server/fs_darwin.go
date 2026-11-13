package main

import (
	"golang.org/x/sys/unix"
	"os"
	"time"
)

func mtime(s *unix.Stat_t) time.Time { return time.Unix(s.Mtim.Sec, s.Mtim.Nsec) }
func renameExclusive(d *os.File, from, to string) error {
	return unix.RenameatxNp(int(d.Fd()), from, int(d.Fd()), to, unix.RENAME_EXCL)
}
