package testutils

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type TestConsoleWriter struct {
	Lines []string
}

func (w *TestConsoleWriter) String() string {
	return strings.Join(w.Lines, "\n")
}

func (w *TestConsoleWriter) Println(a ...any) {
	s := fmt.Sprintln(a...)
	w.Lines = append(w.Lines, s[:len(s)-1]) // remove newline
}

func (w *TestConsoleWriter) Print(a ...any) {
	w.Println(a...)
}

func VerifyStdout(t *testing.T, consoleWriter *TestConsoleWriter, expectedLines ...string) {
	joined := consoleWriter.String()
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}

func VerifyStdoutNotExists(t *testing.T, consoleWriter *TestConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.Lines, expectedLine)
	}
}

func VerifyStdoutEventually(t *testing.T, exec func() *TestConsoleWriter, expectedLines ...string) {
	VerifyStdoutEventuallyWithTimeout(t, exec, WaitDuration, WaitTick, expectedLines...)
}

func VerifyStdoutEventuallyWithTimeout(t *testing.T, exec func() *TestConsoleWriter, waitFor time.Duration, tick time.Duration, expectedLines ...string) {
	require.Eventually(t, func() bool {
		joined := strings.Join(exec().Lines, "\n")
		res := true
		for _, expectedLine := range expectedLines {
			res = res && strings.Contains(joined, expectedLine)
		}
		return res
	}, waitFor, tick)
}
