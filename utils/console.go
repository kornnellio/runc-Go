// Package utils provides console/PTY handling.
package utils

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// ValidateSocketPath checks that a socket path is safe.
func ValidateSocketPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path cannot be empty")
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid socket path: %w", err)
	}

	// Check if path exists and is a socket
	info, err := os.Stat(absPath)
	if err != nil {
		// Path doesn't exist yet - that's fine for sockets being created
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot stat socket path: %w", err)
	}

	// If path exists, it must be a socket
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("path %q exists but is not a socket", path)
	}

	return nil
}

// Console represents a pseudoterminal pair.
type Console struct {
	master *os.File
	slave  *os.File
	path   string
}

// NewConsole creates a new pseudoterminal pair.
func NewConsole() (*Console, error) {
	// Open master PTY
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	// Get slave PTY number
	var ptyno uint32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyno)))
	if errno != 0 {
		master.Close()
		return nil, fmt.Errorf("TIOCGPTN: %v", errno)
	}

	// Unlock slave PTY
	var unlock int32 = 0
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL,
		master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))
	if errno != 0 {
		master.Close()
		return nil, fmt.Errorf("TIOCSPTLCK: %v", errno)
	}

	slavePath := fmt.Sprintf("/dev/pts/%d", ptyno)

	return &Console{
		master: master,
		path:   slavePath,
	}, nil
}

// Master returns the master end of the PTY.
func (c *Console) Master() *os.File {
	return c.master
}

// SlavePath returns the path to the slave PTY.
func (c *Console) SlavePath() string {
	return c.path
}

// OpenSlave opens the slave end of the PTY.
func (c *Console) OpenSlave() (*os.File, error) {
	if c.slave != nil {
		return c.slave, nil
	}

	slave, err := os.OpenFile(c.path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open slave: %w", err)
	}
	c.slave = slave
	return slave, nil
}

// Close closes the console.
func (c *Console) Close() {
	if c.master != nil {
		c.master.Close()
	}
	if c.slave != nil {
		c.slave.Close()
	}
}

// SetControllingTerminal sets the given file as the controlling terminal.
func SetControllingTerminal(f *os.File) error {
	// TIOCSCTTY with arg 1 steals the terminal even if we're not the session leader
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TIOCSCTTY, 1)
	if errno != 0 {
		return fmt.Errorf("TIOCSCTTY: %v", errno)
	}
	return nil
}

// Winsize represents terminal window size.
type Winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// GetWinsize gets the terminal window size.
func GetWinsize(f *os.File) (*Winsize, error) {
	var ws Winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return nil, fmt.Errorf("TIOCGWINSZ: %v", errno)
	}
	return &ws, nil
}

// SetWinsize sets the terminal window size.
func SetWinsize(f *os.File, ws *Winsize) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return fmt.Errorf("TIOCSWINSZ: %v", errno)
	}
	return nil
}

// SendConsoleToSocket sends the console master FD over a unix socket.
// This is used for the --console-socket option.
func SendConsoleToSocket(socketPath string, master *os.File) error {
	// Validate socket path before connecting
	if err := ValidateSocketPath(socketPath); err != nil {
		return fmt.Errorf("invalid console socket: %w", err)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial %s: %w", socketPath, err)
	}
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix connection")
	}

	// Get file for SCM_RIGHTS
	file, err := unixConn.File()
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}
	defer file.Close()

	// Create SCM_RIGHTS message
	rights := syscall.UnixRights(int(master.Fd()))
	if err := syscall.Sendmsg(int(file.Fd()), []byte{0}, rights, nil, 0); err != nil {
		return fmt.Errorf("sendmsg: %w", err)
	}

	return nil
}

// SetRawMode puts the terminal in raw mode.
func SetRawMode(f *os.File) (*syscall.Termios, error) {
	var oldState syscall.Termios

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&oldState)))
	if errno != 0 {
		return nil, fmt.Errorf("TCGETS: %v", errno)
	}

	newState := oldState
	// Input flags: no break, no CR to NL, no parity check, no strip, no flow control
	newState.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	// Output flags: no post processing
	newState.Oflag &^= syscall.OPOST
	// Control flags: set 8-bit chars
	newState.Cflag |= syscall.CS8
	// Local flags: no echo, no canonical, no extended functions, no signal chars
	newState.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	// Control chars: read returns after 1 char, no timeout
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&newState)))
	if errno != 0 {
		return nil, fmt.Errorf("TCSETS: %v", errno)
	}

	return &oldState, nil
}

// RestoreMode restores terminal mode.
func RestoreMode(f *os.File, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(state)))
	if errno != 0 {
		return fmt.Errorf("TCSETS: %v", errno)
	}
	return nil
}

// SetupTerminalSignals ensures the terminal has ISIG enabled for signal generation
// and sets the current process as the foreground process group.
func SetupTerminalSignals(f *os.File) error {
	// Get current terminal settings
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return fmt.Errorf("TCGETS: %v", errno)
	}

	// Enable ISIG to generate signals from Ctrl+C, Ctrl+Z, etc.
	termios.Lflag |= syscall.ISIG

	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return fmt.Errorf("TCSETS: %v", errno)
	}

	// Set ourselves as the foreground process group
	pgrp := syscall.Getpgrp()
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL,
		f.Fd(), syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&pgrp)))
	if errno != 0 {
		// Non-fatal - may fail if not session leader
		return nil
	}

	return nil
}
