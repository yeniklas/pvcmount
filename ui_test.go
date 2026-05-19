package main

import "testing"

func TestStyleFunctions_noTTY(t *testing.T) {
	orig := isTTY
	isTTY = false
	defer func() { isTTY = orig }()

	for _, fn := range []struct {
		name string
		fn   func(string) string
	}{
		{"styleBold", styleBold},
		{"styleDim", styleDim},
		{"styleGreen", styleGreen},
	} {
		got := fn.fn("hello")
		if got != "hello" {
			t.Errorf("%s(\"hello\") without TTY = %q, want plain string", fn.name, got)
		}
	}
}

func TestStyleFunctions_withTTY(t *testing.T) {
	orig := isTTY
	isTTY = true
	defer func() { isTTY = orig }()

	cases := []struct {
		name   string
		fn     func(string) string
		prefix string
	}{
		{"styleBold", styleBold, ansiBold},
		{"styleDim", styleDim, ansiDim},
		{"styleGreen", styleGreen, ansiGreen},
	}

	for _, tt := range cases {
		got := tt.fn("hello")
		want := tt.prefix + "hello" + ansiReset
		if got != want {
			t.Errorf("%s(\"hello\") with TTY = %q, want %q", tt.name, got, want)
		}
	}
}
