//go:build !windows

package main

import "io"

func enableTerminalOutput(io.Writer) (func(), error) { return func() {}, nil }
