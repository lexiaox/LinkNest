package httpapi

import (
	"testing"

	"linknest/server/internal/device"
)

func TestOnlineDevicesFiltersNonOnlineRecords(t *testing.T) {
	items := []device.Device{
		{DeviceID: "online-1", Status: "online"},
		{DeviceID: "offline-1", Status: "offline"},
		{DeviceID: "online-2", Status: " ONLINE "},
		{DeviceID: "empty", Status: ""},
	}

	got := onlineDevices(items)
	if len(got) != 2 {
		t.Fatalf("len(onlineDevices) = %d, want 2", len(got))
	}
	if got[0].DeviceID != "online-1" || got[1].DeviceID != "online-2" {
		t.Fatalf("onlineDevices = %#v, want only online records in original order", got)
	}
}
