package device

import "testing"

func TestOnlineOnlyFiltersAndPreservesOrder(t *testing.T) {
	items := []RemoteDevice{
		{DeviceID: "offline", Status: "offline"},
		{DeviceID: "online-1", Status: "online"},
		{DeviceID: "unknown", Status: "unknown"},
		{DeviceID: "online-2", Status: " ONLINE "},
		{DeviceID: "empty"},
	}

	got := OnlineOnly(items)

	if len(got) != 2 {
		t.Fatalf("len(OnlineOnly()) = %d, want 2", len(got))
	}
	if got[0].DeviceID != "online-1" || got[1].DeviceID != "online-2" {
		t.Fatalf("OnlineOnly() order = [%s %s], want [online-1 online-2]", got[0].DeviceID, got[1].DeviceID)
	}
}

func TestOnlineOnlyEmptyInput(t *testing.T) {
	if got := OnlineOnly(nil); got != nil {
		t.Fatalf("OnlineOnly(nil) = %#v, want nil", got)
	}

	empty := []RemoteDevice{}
	if got := OnlineOnly(empty); len(got) != 0 {
		t.Fatalf("len(OnlineOnly(empty)) = %d, want 0", len(got))
	}
}
