package types

import "fmt"

type (
	ConsoleWrapper interface {
		Println(a ...any)
		Print(a ...any)
	}

	StdoutWrapper struct {
	}
)

func NewStdoutWriter() ConsoleWrapper {
	return &StdoutWrapper{}
}

func (w *StdoutWrapper) Println(a ...any) {
	fmt.Println(a...)
}

func (w *StdoutWrapper) Print(a ...any) {
	fmt.Print(a...)
}
