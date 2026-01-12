// Package linux provides Linux capability management.
package linux

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"runc-go/spec"
)

// Capability constants (from linux/capability.h)
const (
	CAP_CHOWN              = 0
	CAP_DAC_OVERRIDE       = 1
	CAP_DAC_READ_SEARCH    = 2
	CAP_FOWNER             = 3
	CAP_FSETID             = 4
	CAP_KILL               = 5
	CAP_SETGID             = 6
	CAP_SETUID             = 7
	CAP_SETPCAP            = 8
	CAP_LINUX_IMMUTABLE    = 9
	CAP_NET_BIND_SERVICE   = 10
	CAP_NET_BROADCAST      = 11
	CAP_NET_ADMIN          = 12
	CAP_NET_RAW            = 13
	CAP_IPC_LOCK           = 14
	CAP_IPC_OWNER          = 15
	CAP_SYS_MODULE         = 16
	CAP_SYS_RAWIO          = 17
	CAP_SYS_CHROOT         = 18
	CAP_SYS_PTRACE         = 19
	CAP_SYS_PACCT          = 20
	CAP_SYS_ADMIN          = 21
	CAP_SYS_BOOT           = 22
	CAP_SYS_NICE           = 23
	CAP_SYS_RESOURCE       = 24
	CAP_SYS_TIME           = 25
	CAP_SYS_TTY_CONFIG     = 26
	CAP_MKNOD              = 27
	CAP_LEASE              = 28
	CAP_AUDIT_WRITE        = 29
	CAP_AUDIT_CONTROL      = 30
	CAP_SETFCAP            = 31
	CAP_MAC_OVERRIDE       = 32
	CAP_MAC_ADMIN          = 33
	CAP_SYSLOG             = 34
	CAP_WAKE_ALARM         = 35
	CAP_BLOCK_SUSPEND      = 36
	CAP_AUDIT_READ         = 37
	CAP_PERFMON            = 38
	CAP_BPF                = 39
	CAP_CHECKPOINT_RESTORE = 40
)

var (
	// lastCapOnce ensures we only detect the last capability once
	lastCapOnce sync.Once
	// lastCapValue holds the detected last capability value
	lastCapValue int = 40 // default fallback
)

// getLastCap returns the highest capability supported by the kernel.
// This is detected dynamically to support newer kernels with more capabilities.
func getLastCap() int {
	lastCapOnce.Do(func() {
		// Try to read from /proc/sys/kernel/cap_last_cap first (most reliable)
		if data, err := os.ReadFile("/proc/sys/kernel/cap_last_cap"); err == nil {
			if val, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && val >= 0 {
				lastCapValue = val
				return
			}
		}

		// Fallback: probe using prctl
		// Start from known CAP_CHECKPOINT_RESTORE and probe higher
		for cap := 40; cap <= 63; cap++ {
			ret, _, _ := syscall.Syscall(syscall.SYS_PRCTL, PR_CAPBSET_READ, uintptr(cap), 0)
			if ret == ^uintptr(0) { // -1 means EINVAL, cap doesn't exist
				lastCapValue = cap - 1
				return
			}
		}
		lastCapValue = 63 // maximum possible
	})
	return lastCapValue
}

// capabilityMap maps capability names to numbers.
var capabilityMap = map[string]int{
	"CAP_CHOWN":              CAP_CHOWN,
	"CAP_DAC_OVERRIDE":       CAP_DAC_OVERRIDE,
	"CAP_DAC_READ_SEARCH":    CAP_DAC_READ_SEARCH,
	"CAP_FOWNER":             CAP_FOWNER,
	"CAP_FSETID":             CAP_FSETID,
	"CAP_KILL":               CAP_KILL,
	"CAP_SETGID":             CAP_SETGID,
	"CAP_SETUID":             CAP_SETUID,
	"CAP_SETPCAP":            CAP_SETPCAP,
	"CAP_LINUX_IMMUTABLE":    CAP_LINUX_IMMUTABLE,
	"CAP_NET_BIND_SERVICE":   CAP_NET_BIND_SERVICE,
	"CAP_NET_BROADCAST":      CAP_NET_BROADCAST,
	"CAP_NET_ADMIN":          CAP_NET_ADMIN,
	"CAP_NET_RAW":            CAP_NET_RAW,
	"CAP_IPC_LOCK":           CAP_IPC_LOCK,
	"CAP_IPC_OWNER":          CAP_IPC_OWNER,
	"CAP_SYS_MODULE":         CAP_SYS_MODULE,
	"CAP_SYS_RAWIO":          CAP_SYS_RAWIO,
	"CAP_SYS_CHROOT":         CAP_SYS_CHROOT,
	"CAP_SYS_PTRACE":         CAP_SYS_PTRACE,
	"CAP_SYS_PACCT":          CAP_SYS_PACCT,
	"CAP_SYS_ADMIN":          CAP_SYS_ADMIN,
	"CAP_SYS_BOOT":           CAP_SYS_BOOT,
	"CAP_SYS_NICE":           CAP_SYS_NICE,
	"CAP_SYS_RESOURCE":       CAP_SYS_RESOURCE,
	"CAP_SYS_TIME":           CAP_SYS_TIME,
	"CAP_SYS_TTY_CONFIG":     CAP_SYS_TTY_CONFIG,
	"CAP_MKNOD":              CAP_MKNOD,
	"CAP_LEASE":              CAP_LEASE,
	"CAP_AUDIT_WRITE":        CAP_AUDIT_WRITE,
	"CAP_AUDIT_CONTROL":      CAP_AUDIT_CONTROL,
	"CAP_SETFCAP":            CAP_SETFCAP,
	"CAP_MAC_OVERRIDE":       CAP_MAC_OVERRIDE,
	"CAP_MAC_ADMIN":          CAP_MAC_ADMIN,
	"CAP_SYSLOG":             CAP_SYSLOG,
	"CAP_WAKE_ALARM":         CAP_WAKE_ALARM,
	"CAP_BLOCK_SUSPEND":      CAP_BLOCK_SUSPEND,
	"CAP_AUDIT_READ":         CAP_AUDIT_READ,
	"CAP_PERFMON":            CAP_PERFMON,
	"CAP_BPF":                CAP_BPF,
	"CAP_CHECKPOINT_RESTORE": CAP_CHECKPOINT_RESTORE,
}

// prctl constants
const (
	PR_CAPBSET_READ      = 23
	PR_CAPBSET_DROP      = 24
	PR_CAP_AMBIENT       = 47
	PR_CAP_AMBIENT_RAISE = 2
	PR_CAP_AMBIENT_LOWER = 3
	PR_CAP_AMBIENT_CLEAR = 4
)

// Capability header and data structures
const LINUX_CAPABILITY_VERSION_3 = 0x20080522

type capHeader struct {
	Version uint32
	Pid     int32
}

type capData struct {
	Effective   uint32
	Permitted   uint32
	Inheritable uint32
}

// ApplyCapabilities applies OCI capability configuration.
func ApplyCapabilities(caps *spec.LinuxCapabilities) error {
	if caps == nil {
		return nil
	}

	// First, clear ambient capabilities
	clearAmbient()

	// Drop capabilities not in bounding set
	if err := applyBounding(caps.Bounding); err != nil {
		return fmt.Errorf("apply bounding: %w", err)
	}

	// Get current capabilities
	header := capHeader{Version: LINUX_CAPABILITY_VERSION_3, Pid: 0}
	data := [2]capData{}

	// Set effective, permitted, inheritable
	setCapBits(&data, caps.Effective, func(d *capData, idx int, mask uint32) { d.Effective |= mask })
	setCapBits(&data, caps.Permitted, func(d *capData, idx int, mask uint32) { d.Permitted |= mask })
	setCapBits(&data, caps.Inheritable, func(d *capData, idx int, mask uint32) { d.Inheritable |= mask })

	// Apply capabilities
	_, _, errno := syscall.Syscall(syscall.SYS_CAPSET,
		uintptr(unsafe.Pointer(&header)),
		uintptr(unsafe.Pointer(&data[0])),
		0)
	if errno != 0 {
		return fmt.Errorf("capset: %v", errno)
	}

	// Set ambient capabilities (must be both permitted and inheritable)
	if err := applyAmbient(caps.Ambient, caps.Permitted, caps.Inheritable); err != nil {
		return fmt.Errorf("apply ambient: %w", err)
	}

	return nil
}

// clearAmbient clears all ambient capabilities.
func clearAmbient() {
	syscall.Syscall(syscall.SYS_PRCTL, PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR, 0)
}

// applyBounding drops capabilities not in the bounding list.
func applyBounding(bounding []string) error {
	// Build set of allowed capabilities
	allowed := make(map[int]bool)
	for _, name := range bounding {
		capName := strings.ToUpper(name)
		if cap, ok := capabilityMap[capName]; ok {
			allowed[cap] = true
		} else {
			// Warn about unknown capability instead of silently ignoring
			fmt.Printf("[capabilities] warning: unknown capability %q\n", name)
		}
	}

	// Use dynamic last capability detection
	lastCap := getLastCap()

	// Drop all capabilities not in allowed set
	for cap := 0; cap <= lastCap; cap++ {
		if !allowed[cap] {
			// Check if in bounding set first
			ret, _, _ := syscall.Syscall(syscall.SYS_PRCTL, PR_CAPBSET_READ, uintptr(cap), 0)
			if ret == 1 {
				// In bounding set, drop it
				_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, PR_CAPBSET_DROP, uintptr(cap), 0)
				if errno != 0 && errno != syscall.EINVAL {
					return fmt.Errorf("drop cap %d: %v", cap, errno)
				}
			}
		}
	}

	return nil
}

// setCapBits sets capability bits in the data structure.
func setCapBits(data *[2]capData, caps []string, setter func(*capData, int, uint32)) {
	for _, name := range caps {
		if cap, ok := capabilityMap[strings.ToUpper(name)]; ok {
			idx := cap / 32
			bit := uint32(1 << (cap % 32))
			if idx < 2 {
				setter(&data[idx], idx, bit)
			}
		}
	}
}

// applyAmbient sets ambient capabilities.
func applyAmbient(ambient, permitted, inheritable []string) error {
	// Build sets for checking
	permSet := makeCapSet(permitted)
	inhSet := makeCapSet(inheritable)

	for _, name := range ambient {
		cap, ok := capabilityMap[strings.ToUpper(name)]
		if !ok {
			continue
		}

		// Ambient caps must be in both permitted and inheritable
		if !permSet[cap] || !inhSet[cap] {
			continue
		}

		_, _, errno := syscall.Syscall(syscall.SYS_PRCTL,
			PR_CAP_AMBIENT, PR_CAP_AMBIENT_RAISE, uintptr(cap))
		if errno != 0 && errno != syscall.EINVAL {
			return fmt.Errorf("raise ambient cap %d: %v", cap, errno)
		}
	}

	return nil
}

// makeCapSet creates a set of capability numbers from names.
func makeCapSet(caps []string) map[int]bool {
	set := make(map[int]bool)
	for _, name := range caps {
		if cap, ok := capabilityMap[strings.ToUpper(name)]; ok {
			set[cap] = true
		}
	}
	return set
}

// GetCapabilities returns current capability sets.
func GetCapabilities() (effective, permitted, inheritable uint64, err error) {
	header := capHeader{Version: LINUX_CAPABILITY_VERSION_3, Pid: 0}
	data := [2]capData{}

	_, _, errno := syscall.Syscall(syscall.SYS_CAPGET,
		uintptr(unsafe.Pointer(&header)),
		uintptr(unsafe.Pointer(&data[0])),
		0)
	if errno != 0 {
		return 0, 0, 0, fmt.Errorf("capget: %v", errno)
	}

	effective = uint64(data[0].Effective) | (uint64(data[1].Effective) << 32)
	permitted = uint64(data[0].Permitted) | (uint64(data[1].Permitted) << 32)
	inheritable = uint64(data[0].Inheritable) | (uint64(data[1].Inheritable) << 32)

	return effective, permitted, inheritable, nil
}

// CapabilityToName converts a capability number to its name.
func CapabilityToName(cap int) string {
	for name, num := range capabilityMap {
		if num == cap {
			return name
		}
	}
	return fmt.Sprintf("CAP_%d", cap)
}

// NameToCapability converts a capability name to its number.
func NameToCapability(name string) (int, bool) {
	cap, ok := capabilityMap[strings.ToUpper(name)]
	return cap, ok
}

// AllCapabilities returns all known capability names.
func AllCapabilities() []string {
	caps := make([]string, 0, len(capabilityMap))
	for name := range capabilityMap {
		caps = append(caps, name)
	}
	return caps
}
