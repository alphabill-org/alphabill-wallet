package logger

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/lmittmann/tint"
	"github.com/neilotoole/slogt"
)

/*
New returns logger for test t on debug level.
*/
func New(t testing.TB) *slog.Logger {
	return NewLvl(t, slog.LevelDebug)
}

/*
NewLvl returns logger for test t on level "level".

First part of the log line is source location which is invalid for messages logged by the
logger (they are correct for the t.Log, t.Error etc calls). Fix needs support from the Go
testing lib (see https://github.com/golang/go/issues/59928).
*/
func NewLvl(t testing.TB, level slog.Level) *slog.Logger {
	cfg := defaultLogCfg()
	cfg.Level = level.String()
	return newLogger(t, cfg)
}

func newLogger(t testing.TB, cfg types.LogConfiguration) *slog.Logger {
	opt := slogt.Factory(func(w io.Writer) slog.Handler {
		return tint.NewHandler(w, &tint.Options{
			Level:       cfg.LogLevel(),
			NoColor:     cfg.NoColor,
			TimeFormat:  cfg.TimeFormat,
			AddSource:   false,
		})
	})
	return slogt.New(t, opt)
}

func defaultLogCfg() types.LogConfiguration {
	lvl := os.Getenv("AB_TEST_LOG_LEVEL")
	if lvl == "" {
		lvl = slog.LevelDebug.String()
	}
	return types.LogConfiguration{
		Level:      lvl,
		Format:     "console",
		TimeFormat: "15:04:05.0000",
		// slogt is logging into bytes.Buffer so can't use w to detect
		// is the destination console or not (ie for color support).
		// So by default use colors unless env var disables it.
		NoColor:    os.Getenv("AB_TEST_LOG_NO_COLORS") == "true",
	}
}
