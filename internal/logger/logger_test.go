package logger_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/phuslu/log"

	"github.com/shuvo-paul/baaku/internal/logger"
)

func TestNewLevelFromEnv(t *testing.T) {
	cases := []struct{ env, want string }{
		{"", log.InfoLevel.String()},
		{"debug", log.DebugLevel.String()},
		{"warn", log.WarnLevel.String()},
		{"error", log.ErrorLevel.String()},
		{"bogus", log.InfoLevel.String()},
	}
	for _, c := range cases {
		t.Setenv("LOG_LEVEL", c.env)
		if got := logger.New().Level.String(); got != c.want {
			t.Errorf("LOG_LEVEL=%q: level = %q, want %q", c.env, got, c.want)
		}
	}
}

func TestNewLevelFiltering(t *testing.T) {
	t.Setenv("LOG_LEVEL", "info")
	var buf bytes.Buffer
	l := logger.New()
	l.Writer = &log.ConsoleWriter{Writer: &buf, ColorOutput: false}

	l.Debug().Msg("hidden")
	l.Info().Str("k", "v").Msg("shown")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("debug entry leaked below info level: %s", out)
	}
	if !strings.Contains(out, "shown") || !strings.Contains(out, "k=v") {
		t.Errorf("info entry missing from output: %s", out)
	}
}
