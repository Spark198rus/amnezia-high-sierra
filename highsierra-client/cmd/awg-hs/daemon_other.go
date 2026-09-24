//go:build !darwin

package main

import "errors"

func runDaemon(args []string) error {
	return errors.New("the daemon only runs on macOS")
}
