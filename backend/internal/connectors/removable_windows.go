//go:build windows

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type WindowsUSBEnumerator struct{}

func NewWindowsUSBEnumerator() *WindowsUSBEnumerator {
	return &WindowsUSBEnumerator{}
}

func (enumerator *WindowsUSBEnumerator) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	script := `$disks = Get-CimInstance Win32_DiskDrive | Where-Object { ` +
		`$_.InterfaceType -eq 'USB' -or ` +
		`$_.MediaType -like '*External*' -or ` +
		`$_.Model -like '*USB*' -or ` +
		`$_.Model -like '*Portable*' -or ` +
		`$_.PNPDeviceID -like 'USBSTOR*' ` +
		`};` +
		`$items = foreach ($disk in $disks) {` +
		`$partitions = Get-CimAssociatedInstance -InputObject $disk -Association Win32_DiskDriveToDiskPartition;` +
		`foreach ($partition in $partitions) {` +
		`$logicalDisks = Get-CimAssociatedInstance -InputObject $partition -Association Win32_LogicalDiskToPartition;` +
		`foreach ($logical in $logicalDisks) {` +
		`if ($logical.DeviceID -eq $null) { continue }` +
		`[pscustomobject]@{` +
		`mountPoint = "$($logical.DeviceID)\\";` +
		`volumeLabel = $logical.VolumeName;` +
		`fileSystem = $logical.FileSystem;` +
		`volumeSerialNumber = $logical.VolumeSerialNumber;` +
		`driveType = [int]$logical.DriveType;` +
		`interfaceType = $disk.InterfaceType;` +
		`model = $disk.Model;` +
		`pnpDeviceId = $disk.PNPDeviceID` +
		`}` +
		`}` +
		`}` +
		`};` +
		`$items = $items | Sort-Object mountPoint -Unique;` +
		`$items | ConvertTo-Json -Compress`

	command := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	output, err := command.Output()
	if err != nil {
		return nil, newConnectorError(EndpointTypeRemovable, "device_enumeration", ErrorCodeUnavailable, "unable to enumerate removable or external drives", true, err)
	}

	if len(output) == 0 || string(output) == "null" {
		return []DeviceInfo{}, nil
	}

	var devices []DeviceInfo
	if err := json.Unmarshal(output, &devices); err == nil {
		return devices, nil
	}

	var single DeviceInfo
	if err := json.Unmarshal(output, &single); err == nil {
		return []DeviceInfo{single}, nil
	}

	return nil, fmt.Errorf("parse removable devices response: %s", string(output))
}
