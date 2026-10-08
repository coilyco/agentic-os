//go:build !darwin && !linux

package main

func platformTCPOwners(int, int) ([]int, error) { return nil, errNoSocketTable }
