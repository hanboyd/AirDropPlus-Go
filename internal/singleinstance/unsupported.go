//go:build !windows

package singleinstance

func Acquire(string) (func(), error) { return func() {}, nil }
