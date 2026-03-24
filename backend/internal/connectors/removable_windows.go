//go:build windows

package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

type WindowsUSBEnumerator struct{}

func NewWindowsUSBEnumerator() *WindowsUSBEnumerator {
	return &WindowsUSBEnumerator{}
}

func (enumerator *WindowsUSBEnumerator) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	script := `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8;` +
		`$OutputEncoding = [System.Text.Encoding]::UTF8;` +
		`$ProgressPreference = 'SilentlyContinue';` +
		`$ErrorActionPreference = 'Stop';` +
		`function New-DriveRecord($mountPoint, $volumeLabel, $fileSystem, $volumeSerialNumber, $driveType, $interfaceType, $model, $pnpDeviceId) { ` +
		`[pscustomobject]@{` +
		`mountPoint = $mountPoint;` +
		`volumeLabel = $volumeLabel;` +
		`fileSystem = $fileSystem;` +
		`volumeSerialNumber = $volumeSerialNumber;` +
		`driveType = [int]$driveType;` +
		`interfaceType = $interfaceType;` +
		`model = $model;` +
		`pnpDeviceId = $pnpDeviceId` +
		`}` +
		`};` +
		`function Get-UsbLikeDevices() { ` +
		`$result = @();` +
		`$disks = Get-CimInstance Win32_DiskDrive | Where-Object { ` +
		`$_.InterfaceType -eq 'USB' -or ` +
		`$_.MediaType -like '*External*' -or ` +
		`$_.Model -like '*USB*' -or ` +
		`$_.Model -like '*Portable*' -or ` +
		`$_.PNPDeviceID -like 'USBSTOR*' ` +
		`};` +
		`foreach ($disk in $disks) { ` +
		`$partitions = Get-CimAssociatedInstance -InputObject $disk -Association Win32_DiskDriveToDiskPartition;` +
		`foreach ($partition in $partitions) { ` +
		`$logicalDisks = Get-CimAssociatedInstance -InputObject $partition -Association Win32_LogicalDiskToPartition;` +
		`foreach ($logical in $logicalDisks) { ` +
		`if (-not $logical.DeviceID) { continue } ` +
		`$result += New-DriveRecord "$($logical.DeviceID)\\" $logical.VolumeName $logical.FileSystem $logical.VolumeSerialNumber $logical.DriveType $disk.InterfaceType $disk.Model $disk.PNPDeviceID;` +
		`} ` +
		`} ` +
		`} ` +
		`return @($result | Sort-Object mountPoint -Unique);` +
		`};` +
		`function Get-FallbackDevices() { ` +
		`$systemDrive = $null;` +
		`$currentDrive = $null;` +
		`try { $systemDrive = [System.IO.Path]::GetPathRoot($env:SystemRoot) } catch {}` +
		`try { $currentDrive = [System.IO.Path]::GetPathRoot((Get-Location).Path) } catch {}` +
		`$drives = [System.IO.DriveInfo]::GetDrives() | Where-Object { ` +
		`$_.IsReady -and (` +
		`$_.DriveType -eq [System.IO.DriveType]::Removable -or ` +
		`($_.DriveType -eq [System.IO.DriveType]::Fixed -and $_.Name -ne $systemDrive -and $_.Name -ne $currentDrive)` +
		`) ` +
		`};` +
		`$items = foreach ($drive in $drives) { ` +
		`New-DriveRecord $drive.Name $drive.VolumeLabel $drive.DriveFormat $null ([int]$drive.DriveType) 'Unknown' '' '';` +
		`};` +
		`return @($items | Sort-Object mountPoint -Unique);` +
		`};` +
		`$items = @();` +
		`try { $items = Get-UsbLikeDevices } catch { $items = @() }` +
		`if (-not $items -or $items.Count -eq 0) { $items = Get-FallbackDevices }` +
		`$items | ConvertTo-Json -Compress -Depth 4`

	command := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	output, err := command.Output()
	if err != nil {
		return nil, newConnectorError(EndpointTypeRemovable, "device_enumeration", ErrorCodeUnavailable, "unable to enumerate removable or external drives", true, err)
	}

	normalizedOutput, err := normalizeCommandJSONOutput(output)
	if err != nil {
		return nil, newConnectorError(EndpointTypeRemovable, "device_enumeration", ErrorCodeUnavailable, "unable to normalize removable drive output", true, err)
	}

	if len(normalizedOutput) == 0 || string(normalizedOutput) == "null" {
		return []DeviceInfo{}, nil
	}

	var devices []DeviceInfo
	if err := json.Unmarshal(normalizedOutput, &devices); err == nil {
		return devices, nil
	}

	var single DeviceInfo
	if err := json.Unmarshal(normalizedOutput, &single); err == nil {
		return []DeviceInfo{single}, nil
	}

	return nil, fmt.Errorf("parse removable devices response: %s", string(normalizedOutput))
}

func normalizeCommandJSONOutput(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []byte{}, nil
	}

	text, err := decodeCommandOutputText(trimmed)
	if err != nil {
		return nil, err
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return []byte{}, nil
	}

	for _, marker := range []string{"[", "{", "null"} {
		if marker == "null" {
			index := strings.LastIndex(text, marker)
			if index >= 0 {
				candidate := strings.TrimSpace(text[index:])
				if candidate == "null" {
					return []byte(candidate), nil
				}
			}
			continue
		}

		index := strings.LastIndex(text, marker)
		if index >= 0 {
			return []byte(strings.TrimSpace(text[index:])), nil
		}
	}

	return []byte(text), nil
}

func decodeCommandOutputText(raw []byte) (string, error) {
	switch {
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE:
		return decodeUTF16LE(raw[2:]), nil
	case len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF:
		return decodeUTF16BE(raw[2:]), nil
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		return string(raw[3:]), nil
	case bytes.IndexByte(raw, 0x00) >= 0:
		return decodeUTF16LE(raw), nil
	default:
		return string(raw), nil
	}
}

func decodeUTF16LE(raw []byte) string {
	if len(raw)%2 != 0 {
		raw = raw[:len(raw)-1]
	}
	words := make([]uint16, 0, len(raw)/2)
	for index := 0; index+1 < len(raw); index += 2 {
		words = append(words, uint16(raw[index])|uint16(raw[index+1])<<8)
	}
	return string(utf16.Decode(words))
}

func decodeUTF16BE(raw []byte) string {
	if len(raw)%2 != 0 {
		raw = raw[:len(raw)-1]
	}
	words := make([]uint16, 0, len(raw)/2)
	for index := 0; index+1 < len(raw); index += 2 {
		words = append(words, uint16(raw[index])<<8|uint16(raw[index+1]))
	}
	return string(utf16.Decode(words))
}
