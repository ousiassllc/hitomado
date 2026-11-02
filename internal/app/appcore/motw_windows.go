//go:build windows

package appcore

import "os"

// zoneIdentifier は Mark-of-the-Web の内容（ZoneId=3 はインターネットゾーン）。
const zoneIdentifier = "[ZoneTransfer]\r\nZoneId=3\r\n"

func markInternetZone(path string) error {
	return os.WriteFile(path+":Zone.Identifier", []byte(zoneIdentifier), 0o600)
}
