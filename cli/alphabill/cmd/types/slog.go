package types

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

const (
	LevelTrace slog.Level = slog.LevelDebug - 4
	// levelNone is used internally to disable logging
	levelNone slog.Level = math.MinInt

	// valid output Format values
	fmtTEXT    = "text"
	fmtJSON    = "json"
	fmtCONSOLE = "console"
)

type LogConfiguration struct {
	Level           string
	Format          string
	OutputPath      string
	TimeFormat      string
	NoColor         bool
}

func newLogger(cfg *LogConfiguration) (*slog.Logger, error) {
	out, err := filenameToWriter(cfg.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("creating writer for log output: %w", err)
	}

	h, err := cfg.handler(out)
	if err != nil {
		return nil, fmt.Errorf("creating logger handler: %w", err)
	}
	return slog.New(h), nil
}

func (cfg *LogConfiguration) handler(out io.Writer) (slog.Handler, error) {
	// init defaults for everything still unassigned...
	cfg.initDefaults(out)

	handlerOptions := &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.LogLevel(),
	}

	var h slog.Handler
	switch strings.ToLower(cfg.Format) {
	case fmtTEXT:
		h = slog.NewTextHandler(out, handlerOptions)
	case fmtJSON:
		h = slog.NewJSONHandler(out, handlerOptions)
	case fmtCONSOLE:
		h = tint.NewHandler(out, &tint.Options{
			Level:       cfg.LogLevel(),
			NoColor:     cfg.NoColor,
			TimeFormat:  cfg.TimeFormat,
			AddSource:   false,
		})
	default:
		return nil, fmt.Errorf("unknown log format %q", cfg.Format)
	}

	return h, nil
}

/*
initDefaults assigns default value to the fields which are unassigned.
*/
func (cfg *LogConfiguration) initDefaults(out io.Writer) {
	if cfg.Level == "" {
		cfg.Level = slog.LevelInfo.String()
	}
	if cfg.Format == "" {
		cfg.Format = fmtCONSOLE
	}

	if cfg.TimeFormat == "" {
		switch cfg.Format {
		case fmtCONSOLE:
			cfg.TimeFormat = "15:04:05.0000"
		default:
			cfg.TimeFormat = "2006-01-02T15:04:05.0000Z0700"
		}
	}

	f, ok := out.(interface{ Fd() uintptr })
	cfg.NoColor = !(ok && isatty.IsTerminal(f.Fd()))
}

func (cfg *LogConfiguration) LogLevel() slog.Level {
	if cfg.OutputPath == "discard" || cfg.OutputPath == os.DevNull {
		return levelNone
	}

	switch strings.ToLower(cfg.Level) {
	case "warning":
		return slog.LevelWarn
	case "trace":
		return LevelTrace
	case "none":
		return levelNone
	}

	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(cfg.Level))
	return lvl
}

func filenameToWriter(name string) (io.Writer, error) {
	switch strings.ToLower(name) {
	case "stdout":
		return os.Stdout, nil
	case "stderr", "":
		return os.Stderr, nil
	case "discard", os.DevNull:
		return io.Discard, nil
	default:
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return nil, fmt.Errorf("create dir %q for log output: %w", filepath.Dir(name), err)
		}
		file, err := os.OpenFile(filepath.Clean(name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // -rw-------
		if err != nil {
			return nil, fmt.Errorf("open file %q for log output: %w", name, err)
		}
		return file, nil
	}
}
