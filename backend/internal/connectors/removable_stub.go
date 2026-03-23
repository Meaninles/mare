//go:build !windows

package connectors

import "context"

type WindowsUSBEnumerator struct{}

func NewWindowsUSBEnumerator() *WindowsUSBEnumerator {
	return &WindowsUSBEnumerator{}
}

func (enumerator *WindowsUSBEnumerator) ListDevices(context.Context) ([]DeviceInfo, error) {
	return []DeviceInfo{}, newConnectorError(EndpointTypeRemovable, "device_enumeration", ErrorCodeNotSupported, "windows USB enumeration is only available on Windows", false, nil)
}
