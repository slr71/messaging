package messaging

import (
	"log"
	"os"
)

// Logger defines a logging interface for this module.
type Logger interface {
	Print(args ...any)
	Printf(format string, args ...any)
	Println(args ...any)
}

var (
	// Info level logger. Can be set by other packages. Defaults to writing to
	// os.Stdout.
	Info Logger = log.New(os.Stdout, "", log.Lshortfile)

	// Warn level logger. Can be set by other packages. Defaults to writing to
	// os.Stderr.
	Warn Logger = log.New(os.Stderr, "", log.Lshortfile)

	// Error level logger. Can be set by other packages. Default to writing to
	// os.Stderr.
	Error Logger = log.New(os.Stderr, "", log.Lshortfile)
)
