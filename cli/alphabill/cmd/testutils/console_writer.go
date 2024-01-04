package testutils

import (
	"fmt"
	"strings"
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
