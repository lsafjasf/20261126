package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
)

var errNoCommand = errors.New("no command given")

func unknownCommandError(name string) error {
	return fmt.Errorf("unknown command %q", name)
}

func osStdout() *os.File { return os.Stdout }
func osStderr() *os.File { return os.Stderr }

type threadSafeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newThreadSafeBuffer() *threadSafeBuffer {
	return &threadSafeBuffer{}
}

func (b *threadSafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
