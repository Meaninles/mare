package connectors

import (
	"context"
	"sync"
	"time"
)

type DeviceInfo struct {
	MountPoint         string `json:"mountPoint"`
	VolumeLabel        string `json:"volumeLabel"`
	FileSystem         string `json:"fileSystem"`
	VolumeSerialNumber string `json:"volumeSerialNumber"`
	DriveType          int    `json:"driveType"`
	InterfaceType      string `json:"interfaceType"`
	Model              string `json:"model"`
	PNPDeviceID        string `json:"pnpDeviceId"`
}

type DeviceEventType string

const (
	DeviceEventInserted DeviceEventType = "inserted"
	DeviceEventRemoved  DeviceEventType = "removed"
)

type DeviceEvent struct {
	Type   DeviceEventType `json:"type"`
	Device DeviceInfo      `json:"device"`
}

type DeviceEnumerator interface {
	ListDevices(ctx context.Context) ([]DeviceInfo, error)
}

type RemovableDetector struct {
	enumerator   DeviceEnumerator
	pollInterval time.Duration
	mu           sync.Mutex
	known        map[string]DeviceInfo
}

func NewRemovableDetector(enumerator DeviceEnumerator, pollInterval time.Duration) *RemovableDetector {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	return &RemovableDetector{
		enumerator:   enumerator,
		pollInterval: pollInterval,
		known:        make(map[string]DeviceInfo),
	}
}

func (detector *RemovableDetector) Snapshot(ctx context.Context) ([]DeviceInfo, error) {
	return detector.enumerator.ListDevices(ctx)
}

func (detector *RemovableDetector) Start(ctx context.Context) <-chan DeviceEvent {
	events := make(chan DeviceEvent, 16)

	go func() {
		defer close(events)
		ticker := time.NewTicker(detector.pollInterval)
		defer ticker.Stop()

		detector.poll(ctx, events)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				detector.poll(ctx, events)
			}
		}
	}()

	return events
}

func (detector *RemovableDetector) poll(ctx context.Context, events chan<- DeviceEvent) {
	devices, err := detector.enumerator.ListDevices(ctx)
	if err != nil {
		return
	}

	current := make(map[string]DeviceInfo, len(devices))
	for _, device := range devices {
		current[GenerateDeviceIdentity(device)] = device
	}

	detector.mu.Lock()
	defer detector.mu.Unlock()

	for identity, device := range current {
		if _, exists := detector.known[identity]; !exists {
			events <- DeviceEvent{Type: DeviceEventInserted, Device: device}
		}
	}

	for identity, device := range detector.known {
		if _, exists := current[identity]; !exists {
			events <- DeviceEvent{Type: DeviceEventRemoved, Device: device}
		}
	}

	detector.known = current
}
