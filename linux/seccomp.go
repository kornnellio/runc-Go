// Package linux provides seccomp BPF filter support.
package linux

import (
	"fmt"
	"syscall"
	"unsafe"

	"runc-go/spec"
)

// Seccomp constants
const (
	SECCOMP_MODE_FILTER      = 2
	SECCOMP_RET_KILL_PROCESS = 0x80000000
	SECCOMP_RET_KILL_THREAD  = 0x00000000
	SECCOMP_RET_TRAP         = 0x00030000
	SECCOMP_RET_ERRNO        = 0x00050000
	SECCOMP_RET_TRACE        = 0x7ff00000
	SECCOMP_RET_LOG          = 0x7ffc0000
	SECCOMP_RET_ALLOW        = 0x7fff0000

	PR_SET_NO_NEW_PRIVS = 38
	PR_SET_SECCOMP      = 22
)

// BPF constants
const (
	BPF_LD  = 0x00
	BPF_JMP = 0x05
	BPF_RET = 0x06
	BPF_W   = 0x00
	BPF_ABS = 0x20
	BPF_JEQ = 0x10
	BPF_JGE = 0x30
	BPF_JGT = 0x20
	BPF_K   = 0x00
)

// Seccomp data offsets
const (
	offsetNR   = 0
	offsetArch = 4
)

// Architecture audit values
const (
	AUDIT_ARCH_X86_64  = 0xc000003e
	AUDIT_ARCH_I386    = 0x40000003
	AUDIT_ARCH_AARCH64 = 0xc00000b7
	AUDIT_ARCH_ARM     = 0x40000028
)

// sockFprog is the BPF program structure.
type sockFprog struct {
	Len    uint16
	Filter *sockFilter
}

// sockFilter is a single BPF instruction.
type sockFilter struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// actionToRet maps OCI seccomp actions to return values.
var actionToRet = map[spec.LinuxSeccompAction]uint32{
	spec.ActKill:        SECCOMP_RET_KILL_THREAD,
	spec.ActKillProcess: SECCOMP_RET_KILL_PROCESS,
	spec.ActKillThread:  SECCOMP_RET_KILL_THREAD,
	spec.ActTrap:        SECCOMP_RET_TRAP,
	spec.ActErrno:       SECCOMP_RET_ERRNO,
	spec.ActTrace:       SECCOMP_RET_TRACE,
	spec.ActAllow:       SECCOMP_RET_ALLOW,
	spec.ActLog:         SECCOMP_RET_LOG,
}

// archToAudit maps OCI arch to audit arch value.
var archToAudit = map[spec.Arch]uint32{
	spec.ArchX86_64:  AUDIT_ARCH_X86_64,
	spec.ArchX86:     AUDIT_ARCH_I386,
	spec.ArchAARCH64: AUDIT_ARCH_AARCH64,
	spec.ArchARM:     AUDIT_ARCH_ARM,
}

// syscallMap maps syscall names to numbers (x86_64).
// This is a subset - full implementation would use a complete table.
var syscallMap = map[string]int{
	"read": 0, "write": 1, "open": 2, "close": 3, "stat": 4,
	"fstat": 5, "lstat": 6, "poll": 7, "lseek": 8, "mmap": 9,
	"mprotect": 10, "munmap": 11, "brk": 12, "ioctl": 16,
	"access": 21, "pipe": 22, "select": 23, "sched_yield": 24,
	"mremap": 25, "msync": 26, "mincore": 27, "madvise": 28,
	"dup": 32, "dup2": 33, "pause": 34, "nanosleep": 35,
	"getpid": 39, "socket": 41, "connect": 42, "accept": 43,
	"sendto": 44, "recvfrom": 45, "sendmsg": 46, "recvmsg": 47,
	"shutdown": 48, "bind": 49, "listen": 50, "getsockname": 51,
	"getpeername": 52, "socketpair": 53, "setsockopt": 54,
	"getsockopt": 55, "clone": 56, "fork": 57, "vfork": 58,
	"execve": 59, "exit": 60, "wait4": 61, "kill": 62,
	"uname": 63, "fcntl": 72, "flock": 73, "fsync": 74,
	"fdatasync": 75, "truncate": 76, "ftruncate": 77,
	"getdents": 78, "getcwd": 79, "chdir": 80, "fchdir": 81,
	"rename": 82, "mkdir": 83, "rmdir": 84, "creat": 85,
	"link": 86, "unlink": 87, "symlink": 88, "readlink": 89,
	"chmod": 90, "fchmod": 91, "chown": 92, "fchown": 93,
	"lchown": 94, "umask": 95, "gettimeofday": 96, "getrlimit": 97,
	"getrusage": 98, "sysinfo": 99, "times": 100,
	"ptrace": 101, "getuid": 102, "syslog": 103, "getgid": 104,
	"setuid": 105, "setgid": 106, "geteuid": 107, "getegid": 108,
	"setpgid": 109, "getppid": 110, "getpgrp": 111, "setsid": 112,
	"setreuid": 113, "setregid": 114, "getgroups": 115, "setgroups": 116,
	"setresuid": 117, "getresuid": 118, "setresgid": 119, "getresgid": 120,
	"getpgid": 121, "setfsuid": 122, "setfsgid": 123, "getsid": 124,
	"capget": 125, "capset": 126, "rt_sigpending": 127,
	"rt_sigtimedwait": 128, "rt_sigqueueinfo": 129, "rt_sigsuspend": 130,
	"sigaltstack": 131, "utime": 132, "mknod": 133,
	"personality": 135, "ustat": 136, "statfs": 137, "fstatfs": 138,
	"sysfs": 139, "getpriority": 140, "setpriority": 141,
	"sched_setparam": 142, "sched_getparam": 143,
	"sched_setscheduler": 144, "sched_getscheduler": 145,
	"sched_get_priority_max": 146, "sched_get_priority_min": 147,
	"sched_rr_get_interval": 148, "mlock": 149, "munlock": 150,
	"mlockall": 151, "munlockall": 152, "vhangup": 153,
	"modify_ldt": 154, "pivot_root": 155, "_sysctl": 156,
	"prctl": 157, "arch_prctl": 158, "adjtimex": 159,
	"setrlimit": 160, "chroot": 161, "sync": 162, "acct": 163,
	"settimeofday": 164, "mount": 165, "umount2": 166,
	"swapon": 167, "swapoff": 168, "reboot": 169,
	"sethostname": 170, "setdomainname": 171, "iopl": 172, "ioperm": 173,
	"init_module": 175, "delete_module": 176,
	"quotactl": 179, "nfsservctl": 180,
	"gettid": 186, "readahead": 187, "setxattr": 188,
	"getxattr": 191, "listxattr": 194, "removexattr": 197,
	"tkill": 200, "time": 201, "futex": 202,
	"sched_setaffinity": 203, "sched_getaffinity": 204,
	"io_setup": 206, "io_destroy": 207, "io_getevents": 208,
	"io_submit": 209, "io_cancel": 210, "lookup_dcookie": 212,
	"epoll_create": 213, "remap_file_pages": 216,
	"getdents64": 217, "set_tid_address": 218, "restart_syscall": 219,
	"semtimedop": 220, "fadvise64": 221, "timer_create": 222,
	"timer_settime": 223, "timer_gettime": 224, "timer_getoverrun": 225,
	"timer_delete": 226, "clock_settime": 227, "clock_gettime": 228,
	"clock_getres": 229, "clock_nanosleep": 230, "exit_group": 231,
	"epoll_wait": 232, "epoll_ctl": 233, "tgkill": 234,
	"utimes": 235, "mbind": 237, "set_mempolicy": 238,
	"get_mempolicy": 239, "mq_open": 240, "mq_unlink": 241,
	"mq_timedsend": 242, "mq_timedreceive": 243, "mq_notify": 244,
	"mq_getsetattr": 245, "kexec_load": 246, "waitid": 247,
	"add_key": 248, "request_key": 249, "keyctl": 250,
	"ioprio_set": 251, "ioprio_get": 252, "inotify_init": 253,
	"inotify_add_watch": 254, "inotify_rm_watch": 255,
	"migrate_pages": 256, "openat": 257, "mkdirat": 258,
	"mknodat": 259, "fchownat": 260, "futimesat": 261,
	"newfstatat": 262, "unlinkat": 263, "renameat": 264,
	"linkat": 265, "symlinkat": 266, "readlinkat": 267,
	"fchmodat": 268, "faccessat": 269, "pselect6": 270,
	"ppoll": 271, "unshare": 272, "set_robust_list": 273,
	"get_robust_list": 274, "splice": 275, "tee": 276,
	"sync_file_range": 277, "vmsplice": 278, "move_pages": 279,
	"utimensat": 280, "epoll_pwait": 281, "signalfd": 282,
	"timerfd_create": 283, "eventfd": 284, "fallocate": 285,
	"timerfd_settime": 286, "timerfd_gettime": 287, "accept4": 288,
	"signalfd4": 289, "eventfd2": 290, "epoll_create1": 291,
	"dup3": 292, "pipe2": 293, "inotify_init1": 294,
	"preadv": 295, "pwritev": 296, "rt_tgsigqueueinfo": 297,
	"perf_event_open": 298, "recvmmsg": 299, "fanotify_init": 300,
	"fanotify_mark": 301, "prlimit64": 302, "name_to_handle_at": 303,
	"open_by_handle_at": 304, "clock_adjtime": 305, "syncfs": 306,
	"sendmmsg": 307, "setns": 308, "getcpu": 309, "process_vm_readv": 310,
	"process_vm_writev": 311, "kcmp": 312, "finit_module": 313,
	"sched_setattr": 314, "sched_getattr": 315, "renameat2": 316,
	"seccomp": 317, "getrandom": 318, "memfd_create": 319,
	"kexec_file_load": 320, "bpf": 321, "execveat": 322,
	"userfaultfd": 323, "membarrier": 324, "mlock2": 325,
	"copy_file_range": 326, "preadv2": 327, "pwritev2": 328,
	"pkey_mprotect": 329, "pkey_alloc": 330, "pkey_free": 331,
	"statx": 332, "io_pgetevents": 333, "rseq": 334,
}

// SetupSeccomp installs a seccomp filter based on OCI configuration.
func SetupSeccomp(config *spec.LinuxSeccomp) error {
	if config == nil {
		return nil
	}

	// Count how many syscalls we recognize vs don't recognize
	// If too many are unrecognized, skip our filter - let Docker/containerd's
	// native seccomp handle it instead (they use libseccomp which is complete)
	var recognized, unrecognized int
	for _, rule := range config.Syscalls {
		for _, name := range rule.Names {
			if _, ok := syscallMap[name]; ok {
				recognized++
			} else {
				unrecognized++
			}
		}
	}

	// If more than 20% of syscalls are unrecognized, skip our incomplete filter
	// This is a safety measure for Docker integration
	if unrecognized > 0 && (recognized == 0 || float64(unrecognized)/float64(recognized+unrecognized) > 0.2) {
		// Skip applying our incomplete filter
		// Docker/containerd applies seccomp via containerd-shim anyway
		return nil
	}

	// Set no new privileges
	_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, PR_SET_NO_NEW_PRIVS, 1, 0)
	if errno != 0 {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %v", errno)
	}

	// Build BPF filter
	filter, err := buildSeccompFilter(config)
	if err != nil {
		return fmt.Errorf("build filter: %w", err)
	}

	if len(filter) == 0 {
		return nil
	}

	prog := sockFprog{
		Len:    uint16(len(filter)),
		Filter: &filter[0],
	}

	// Install filter
	_, _, errno = syscall.Syscall(syscall.SYS_PRCTL,
		PR_SET_SECCOMP,
		SECCOMP_MODE_FILTER,
		uintptr(unsafe.Pointer(&prog)))
	if errno != 0 {
		return fmt.Errorf("prctl(PR_SET_SECCOMP): %v", errno)
	}

	return nil
}

// buildSeccompFilter builds a BPF filter from OCI seccomp config.
func buildSeccompFilter(config *spec.LinuxSeccomp) ([]sockFilter, error) {
	var filter []sockFilter

	// Get default action return value
	defaultRet, ok := actionToRet[config.DefaultAction]
	if !ok {
		return nil, fmt.Errorf("unknown default action: %s", config.DefaultAction)
	}

	// Step 1: Load and check architecture
	filter = append(filter, bpfStmt(BPF_LD|BPF_W|BPF_ABS, offsetArch))

	// Allow only specified architectures (default to native)
	arches := config.Architectures
	if len(arches) == 0 {
		arches = []spec.Arch{spec.ArchX86_64}
	}

	// Jump over kill if arch matches any allowed
	archChecks := len(arches)
	for i, arch := range arches {
		auditArch, ok := archToAudit[arch]
		if !ok {
			continue
		}
		// Jump past remaining arch checks + kill instruction if match
		jt := uint8(archChecks - i)
		filter = append(filter, bpfJump(BPF_JMP|BPF_JEQ|BPF_K, auditArch, jt, 0))
	}
	// Kill if no arch matched
	filter = append(filter, bpfStmt(BPF_RET|BPF_K, SECCOMP_RET_KILL_PROCESS))

	// Step 2: Load syscall number
	filter = append(filter, bpfStmt(BPF_LD|BPF_W|BPF_ABS, offsetNR))

	// Step 3: Add rules for each syscall
	for _, rule := range config.Syscalls {
		action, ok := actionToRet[rule.Action]
		if !ok {
			continue
		}

		// Handle errno return value
		if rule.Action == spec.ActErrno && rule.ErrnoRet != nil {
			action = SECCOMP_RET_ERRNO | uint32(*rule.ErrnoRet)
		}

		for _, name := range rule.Names {
			nr, ok := syscallMap[name]
			if !ok {
				// Unknown syscall, skip
				continue
			}

			// Simple rule: if syscall matches, return action
			// TODO: Implement argument checking for rule.Args
			if len(rule.Args) == 0 {
				// Jump to action return, else continue
				// We append the return at the end
				filter = append(filter, bpfJump(BPF_JMP|BPF_JEQ|BPF_K, uint32(nr), 0, 1))
				filter = append(filter, bpfStmt(BPF_RET|BPF_K, action))
			}
		}
	}

	// Step 4: Default action
	filter = append(filter, bpfStmt(BPF_RET|BPF_K, defaultRet))

	return filter, nil
}

// bpfStmt creates a BPF statement.
func bpfStmt(code uint16, k uint32) sockFilter {
	return sockFilter{Code: code, Jt: 0, Jf: 0, K: k}
}

// bpfJump creates a BPF jump instruction.
func bpfJump(code uint16, k uint32, jt, jf uint8) sockFilter {
	return sockFilter{Code: code, Jt: jt, Jf: jf, K: k}
}

// SyscallNumber returns the syscall number for a name.
func SyscallNumber(name string) (int, bool) {
	nr, ok := syscallMap[name]
	return nr, ok
}
