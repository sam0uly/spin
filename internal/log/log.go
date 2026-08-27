// Package log provides the project logger for diagnostics, warnings,
// and errors. Styled user-facing output lives elsewhere; this package
// writes to stderr with colors enabled and no timestamps.
package log

import (
	"os"

	"charm.land/log/v2"
)

// Logger is the stderr logger used for warnings, errors, and
// diagnostics. Prefer the helpers below over calling it directly.
var Logger = log.NewWithOptions(os.Stderr, log.Options{
	Level:           log.InfoLevel,
	ReportTimestamp: false,
})

// Stdout logs user-facing informational output to stdout, sharing its
// settings with Logger.
var Stdout = log.NewWithOptions(os.Stdout, log.Options{
	Level:           log.InfoLevel,
	ReportTimestamp: false,
})

// Info logs an info-level message to stderr.
func Info(msg string, args ...any) { Logger.Info(msg, args...) }

// Warn logs a warning-level message to stderr.
func Warn(msg string, args ...any) { Logger.Warn(msg, args...) }

// Error logs an error-level message to stderr.
func Error(msg string, args ...any) { Logger.Error(msg, args...) }

// Debug logs a debug-level message to stderr.
func Debug(msg string, args ...any) { Logger.Debug(msg, args...) }

// Print logs a message without a level prefix to stderr.
func Print(msg string, args ...any) { Logger.Print(msg, args...) }

// Fatal logs an error-level message to stderr and exits.
func Fatal(msg string, args ...any) { Logger.Fatal(msg, args...) }

// SetLevel changes the current logging level for both loggers.
func SetLevel(l log.Level) {
	Logger.SetLevel(l)
	Stdout.SetLevel(l)
}
