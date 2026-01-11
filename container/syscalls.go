// Package container provides syscall wrappers.
package container

import (
	"syscall"
)

// execProcess executes a process (does not return on success).
func execProcess(path string, args []string, env []string) error {
	return syscall.Exec(path, args, env)
}

// setUid sets the user ID.
func setUid(uid int) error {
	return syscall.Setuid(uid)
}

// setGid sets the group ID.
func setGid(gid int) error {
	return syscall.Setgid(gid)
}

// setUmask sets the umask and returns the old value.
func setUmask(mask int) int {
	return syscall.Umask(mask)
}

// setGroups sets supplementary group IDs.
func setGroups(gids []int) error {
	return syscall.Setgroups(gids)
}
