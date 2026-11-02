//go:build !windows

package appcore

func markInternetZone(string) error { return nil }
