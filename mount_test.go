package main

import "testing"

func TestIsMounted(t *testing.T) {
	// The root filesystem is always mounted.
	if !isMounted("/") {
		t.Error("isMounted(\"/\") = false, want true")
	}

	// A path that cannot be a real mountpoint.
	if isMounted("/this/path/does/not/exist/pvcmount-test") {
		t.Error("isMounted of nonexistent path = true, want false")
	}
}
