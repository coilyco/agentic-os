//go:build !darwin && !linux

package main

import "errors"

func acquireAwake(string) (awakeLock, error) {
	return nil, errors.New("this platform has no keep-awake in aterm")
}
