//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"io"
	"os"
)

func enableTerminalOutput(output io.Writer) (func(), error) {
	file, ok := output.(*os.File)
	if !ok {
		return func() {}, nil
	}
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return func() {}, nil
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|windows.DISABLE_NEWLINE_AUTO_RETURN); err != nil {
		return nil, err
	}
	return func() { windows.SetConsoleMode(handle, mode) }, nil
}
