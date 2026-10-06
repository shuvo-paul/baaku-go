// Package logger configures the phuslu/log logger from the environment:
// human-readable console output locally, JSON to stderr in production.
package logger

import (
	"os"
	"strings"

	"github.com/phuslu/log"
)

// levels maps LOG_LEVEL values; anything unlisted falls back to info.
// (log.ParseLevel returns an unexported noLevel for unknown input, so we
// can't reuse it for validation.)
var levels = map[string]log.Level{
	"trace": log.TraceLevel,
	"debug": log.DebugLevel,
	"info":  log.InfoLevel,
	"warn":  log.WarnLevel,
	"error": log.ErrorLevel,
}

// New builds a stderr logger. APP_ENV=production selects JSON output;
// everything else gets the colored console format. LOG_LEVEL
// (trace/debug/info/warn/error, default info) sets the level.
func New() *log.Logger {
	level, ok := levels[strings.ToLower(os.Getenv("LOG_LEVEL"))]
	if !ok {
		level = log.InfoLevel
	}

	l := &log.Logger{Level: level}
	if os.Getenv("APP_ENV") == "production" {
		// Default writer is already JSON to stderr.
		return l
	}
	l.Writer = &log.ConsoleWriter{ColorOutput: true}
	return l
}
