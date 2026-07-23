package main

import (
	"os"
	"testing"
)

// TestMain isolates the runtime config directory so ambient machine config does
// not leak into shell construction. Tests that need a specific runtime config
// seed their own via t.Setenv("XDG_CONFIG_HOME", ...), which overrides this for
// the duration of that test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "box-dispatch-test-config")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
