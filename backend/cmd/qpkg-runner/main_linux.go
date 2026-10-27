// QPKG hooks run as root; drop privileges before touching runtime files or photos.
package main

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func main() {
	if len(os.Args) != 5 {
		fatal("usage: qpkg-runner USER SCRIPT ACTION PACKAGE_DIR")
	}
	u, err := user.Lookup(os.Args[1])
	if err != nil {
		fatal(err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || uid == 0 {
		fatal("invalid service UID")
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		fatal(err)
	}
	if os.Geteuid() == 0 {
		ids, err := u.GroupIds()
		if err != nil {
			fatal(err)
		}
		groups := make([]int, 0, len(ids))
		for _, id := range ids {
			n, e := strconv.Atoi(id)
			if e != nil {
				fatal(e)
			}
			groups = append(groups, n)
		}
		if err = syscall.Setgroups(groups); err != nil {
			fatal(err)
		}
		if err = syscall.Setgid(gid); err != nil {
			fatal(err)
		}
		if err = syscall.Setuid(uid); err != nil {
			fatal(err)
		}
	}
	if os.Geteuid() != uid {
		fatal("must run as root or the configured service user")
	}
	os.Setenv("HOME", u.HomeDir)
	os.Setenv("USER", u.Username)
	os.Setenv("LOGNAME", u.Username)
	if err = syscall.Exec("/bin/sh", append([]string{"sh"}, os.Args[2:]...), os.Environ()); err != nil {
		fatal(err)
	}
}
func fatal(v interface{}) { fmt.Fprintln(os.Stderr, v); os.Exit(1) }
