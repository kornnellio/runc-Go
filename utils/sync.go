// Package utils provides utility functions for the runtime.
package utils

import (
	"fmt"
	"os"
	"syscall"
)

// SyncPipe is a pipe used for parent-child synchronization.
type SyncPipe struct {
	parent *os.File
	child  *os.File
}

// NewSyncPipe creates a new synchronization pipe.
func NewSyncPipe() (*SyncPipe, error) {
	fds := make([]int, 2)
	if err := syscall.Pipe(fds); err != nil {
		return nil, fmt.Errorf("pipe: %w", err)
	}

	return &SyncPipe{
		parent: os.NewFile(uintptr(fds[0]), "syncpipe-parent"),
		child:  os.NewFile(uintptr(fds[1]), "syncpipe-child"),
	}, nil
}

// ParentFile returns the parent (reading) end of the pipe.
func (s *SyncPipe) ParentFile() *os.File {
	return s.parent
}

// ChildFile returns the child (writing) end of the pipe.
func (s *SyncPipe) ChildFile() *os.File {
	return s.child
}

// CloseParent closes the parent end of the pipe.
func (s *SyncPipe) CloseParent() error {
	if s.parent != nil {
		return s.parent.Close()
	}
	return nil
}

// CloseChild closes the child end of the pipe.
func (s *SyncPipe) CloseChild() error {
	if s.child != nil {
		return s.child.Close()
	}
	return nil
}

// Close closes both ends of the pipe.
func (s *SyncPipe) Close() {
	s.CloseParent()
	s.CloseChild()
}

// Wait waits for a signal on the parent end (blocking read).
func (s *SyncPipe) Wait() error {
	buf := make([]byte, 1)
	_, err := s.parent.Read(buf)
	return err
}

// Signal sends a signal on the child end.
func (s *SyncPipe) Signal() error {
	_, err := s.child.Write([]byte{0})
	return err
}

// WaitWithError waits and returns any error message.
func (s *SyncPipe) WaitWithError() error {
	buf := make([]byte, 1024)
	n, err := s.parent.Read(buf)
	if err != nil {
		return err
	}
	if n > 0 && buf[0] != 0 {
		return fmt.Errorf("%s", string(buf[:n]))
	}
	return nil
}

// SignalError sends an error message.
func (s *SyncPipe) SignalError(err error) error {
	_, writeErr := s.child.Write([]byte(err.Error()))
	return writeErr
}

// Fifo provides FIFO-based synchronization.
type Fifo struct {
	path string
}

// NewFifo creates a new FIFO at the given path.
func NewFifo(path string) (*Fifo, error) {
	// Remove existing FIFO if present
	os.Remove(path)

	if err := syscall.Mkfifo(path, 0600); err != nil {
		return nil, fmt.Errorf("mkfifo %s: %w", path, err)
	}

	return &Fifo{path: path}, nil
}

// OpenFifo opens an existing FIFO.
func OpenFifo(path string) *Fifo {
	return &Fifo{path: path}
}

// Path returns the path to the FIFO.
func (f *Fifo) Path() string {
	return f.path
}

// Wait opens the FIFO for reading and waits for a signal.
func (f *Fifo) Wait() error {
	file, err := os.OpenFile(f.path, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open fifo: %w", err)
	}
	defer file.Close()

	buf := make([]byte, 1)
	_, err = file.Read(buf)
	return err
}

// Signal opens the FIFO for writing and sends a signal.
func (f *Fifo) Signal() error {
	file, err := os.OpenFile(f.path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open fifo: %w", err)
	}
	defer file.Close()

	_, err = file.Write([]byte{0})
	return err
}

// Remove removes the FIFO.
func (f *Fifo) Remove() error {
	return os.Remove(f.path)
}
