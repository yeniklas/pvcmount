package main

import (
	"fmt"
	"os"
)

var debug bool

var isTTY = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}()

const (
	ansiReset = "\033[0m"
	ansiBold  = "\033[1m"
	ansiDim   = "\033[2m"
	ansiGreen = "\033[32m"
)

func styleBold(s string) string {
	if !isTTY {
		return s
	}
	return ansiBold + s + ansiReset
}

func styleDim(s string) string {
	if !isTTY {
		return s
	}
	return ansiDim + s + ansiReset
}

func styleGreen(s string) string {
	if !isTTY {
		return s
	}
	return ansiGreen + s + ansiReset
}

func stepDone(label string) {
	fmt.Printf("  %s %s\n", styleGreen("✓"), label)
}

func debugf(format string, args ...any) {
	if debug {
		fmt.Printf("  "+styleDim(fmt.Sprintf(format, args...))+"\n")
	}
}
