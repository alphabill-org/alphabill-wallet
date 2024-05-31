package logger

import (
	"fmt"
	"log/slog"
	"testing"
)

func Test_logger_for_tests(t *testing.T) {
	t.Skip("this test is only for visually checking the output")

	t.Run("first", func(t *testing.T) {
		l := New(t)
		l.Error("now thats really bad", slog.Any("err", fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})

	t.Run("second", func(t *testing.T) {
		l := NewLvl(t, slog.LevelInfo)
		l.Error("now thats really bad", slog.Any("err", fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		t.Log("this is INFO level logger so Debug call should not show up")
		l.Debug("this shouldn't show up in the log")
		t.Fail()
	})
}

func Test_logger_for_tests_color(t *testing.T) {
	t.Skip("this test is only for visually checking the output")

	t.Run("colors disabled", func(t *testing.T) {
		t.Setenv("AB_TEST_LOG_NO_COLORS", "true")

		l := New(t)
		l.Error("now thats really bad", slog.Any("err", fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})

	t.Run("colors enabled", func(t *testing.T) {
		t.Setenv("AB_TEST_LOG_NO_COLORS", "false")

		l := New(t)
		l.Error("now thats really bad", slog.Any("err", fmt.Errorf("what now")))
		l.Warn("going to tell it just once")
		l.Info("so you know")
		l.Debug("lets investigate")
		t.Error("calling t.Error causes the test to fail")
	})
}
