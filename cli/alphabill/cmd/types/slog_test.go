package types

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

func Test_LogConfiguration_logLevel(t *testing.T) {
	var cases = []struct {
		name  string
		level slog.Level
	}{
		{"", slog.LevelInfo},
		{"error", slog.LevelError},
		{"InfO", slog.LevelInfo},
		{"ERROR", slog.LevelError},
		{"WARNING", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"INFO", slog.LevelInfo},
		{"DEBUG", slog.LevelDebug},
		{"TRACE", LevelTrace},
		{"NONE", levelNone},
		{"info-1", slog.LevelInfo - 1},
		{"info+1", slog.LevelInfo + 1},
		{"foobar", slog.LevelInfo}, // invalid value, expect default level
	}

	for _, tc := range cases {
		cfg := logConfiguration{level: tc.name}
		if lvl := cfg.logLevel(); lvl != tc.level {
			t.Errorf("expected %q to return %d (%s) but got %d (%s)", tc.name, tc.level, tc.level, lvl, lvl)
		}
	}

	// special case - when OutputPath is "discard" return levelNone
	cfg := logConfiguration{level: "info", outputPath: "discard"}
	if lvl := cfg.logLevel(); lvl != levelNone {
		t.Errorf("expected %d but got %d for level", levelNone, lvl)
	}

	cfg = logConfiguration{level: "info", outputPath: os.DevNull}
	if lvl := cfg.logLevel(); lvl != levelNone {
		t.Errorf("expected %d but got %d for level", levelNone, lvl)
	}
}

func Test_loggers_json_output(t *testing.T) {
	log, err := newLogger(&logConfiguration{outputPath: "stdout", level: "debug", format: "json"})
	if err != nil {
		for ; err != nil; err = errors.Unwrap(err) {
			t.Logf("%T : %v", err, err)
		}
		t.Fatalf("initializing logger: %v", err)
	}
	type foo struct {
		V string
	}

	log.LogAttrs(context.Background(), slog.LevelInfo, "a log message in JSON format in stdout",
		slog.Any("data", &foo{"bar"}))

	logDiscard, err := newLogger(&logConfiguration{outputPath: "discard", level: "debug", format: "console"})
	if err != nil {
		for ; err != nil; err = errors.Unwrap(err) {
			t.Logf("%T : %v", err, err)
		}
		t.Fatalf("initializing logger: %v", err)
	}

	logDiscard.Log(context.Background(), slog.LevelInfo, "this log message should not be visible")
}
