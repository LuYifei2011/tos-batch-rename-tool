package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

const tosAppDirectory = "/usr/local/batch-rename"

// Pin the directory before changing ownership so a symlink cannot redirect initialization.
func prepareDataDirectory(app string, uid, gid int) error {
	parent, err := os.Open(app)
	if err != nil {
		return fmt.Errorf("open application directory: %w", err)
	}
	defer parent.Close()
	if err := unix.Mkdirat(int(parent.Fd()), "data", 0700); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("create data directory: %w", err)
	}
	fd, err := unix.Openat(int(parent.Fd()), "data", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("data must be a real directory: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.Fchown(fd, uid, gid); err != nil {
		return fmt.Errorf("set data directory owner: %w", err)
	}
	if err := unix.Fchmod(fd, 0700); err != nil {
		return fmt.Errorf("set data directory permissions: %w", err)
	}
	return nil
}

func prepareTOSData() error {
	if os.Geteuid() != 0 {
		return errors.New("data initialization must be invoked by the system service manager")
	}
	account, err := user.Lookup("batch-rename")
	if err != nil {
		return fmt.Errorf("TOS application account is unavailable: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 {
		return errors.New("TOS application account must have a non-root UID")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid <= 0 {
		return errors.New("TOS application account must have a non-root primary group")
	}
	return prepareDataDirectory(tosAppDirectory, uid, gid)
}

func checkTOSData(data string) error {
	info, err := os.Lstat(data)
	if err != nil {
		return fmt.Errorf("data directory is unavailable; enable the application through TOS: %w", err)
	}
	if !info.IsDir() {
		return errors.New("data must be a directory, not a symbolic link")
	}
	if err := unix.Access(data, unix.R_OK|unix.W_OK|unix.X_OK); err != nil {
		return fmt.Errorf("application account cannot access the data directory: %w", err)
	}
	return nil
}
