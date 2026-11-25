package main

import "io"

// dispatch runs one subcommand. It is split from main so tests can invoke
// commands without spawning a new process.
func dispatch(args []string) error {
	if len(args) == 0 {
		usage()
		return errNoCommand
	}
	switch args[0] {
	case "validate":
		return cmdValidate(args[1:])
	case "map":
		return cmdMap(args[1:])
	case "unresolved":
		return cmdUnresolved(args[1:])
	case "resolve":
		return cmdResolve(args[1:])
	case "anchor":
		return cmdAnchor(args[1:])
	case "unresolve":
		return cmdUnresolve(args[1:])
	case "checksum":
		return cmdChecksum(args[1:])
	case "demo":
		return cmdDemo(args[1:])
	default:
		return unknownCommandError(args[0])
	}
}

// capture swaps package-level output sinks around fn. The commands write via
// outWriter/errWriter instead of os.Stdout/os.Stderr directly.
func capture(fn func() error) (string, error) {
	prevOut, prevErr := outWriter, errWriter
	buf := newThreadSafeBuffer()
	outWriter = buf
	errWriter = buf
	defer func() {
		outWriter = prevOut
		errWriter = prevErr
	}()
	err := fn()
	return buf.String(), err
}

var (
	outWriter io.Writer = osStdout()
	errWriter io.Writer = osStderr()
)
