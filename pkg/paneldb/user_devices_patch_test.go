package paneldb

import "testing"

func TestPatchUserDeviceBindingsPreservesMaxDevices(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := UpsertUser(db, "main", "user1", &User{MaxDevices: 8, Comment: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := PatchUserDeviceBindings(db, "main", "user1", []string{"dev1"}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	u := got.Users["user1"]
	if u == nil {
		t.Fatal("user not found")
	}
	if u.MaxDevices != 8 {
		t.Fatalf("max_devices = %d, want 8 after first bind", u.MaxDevices)
	}
	if len(u.DeviceIDs) != 1 || u.DeviceIDs[0] != "dev1" {
		t.Fatalf("device_ids = %v, want [dev1]", u.DeviceIDs)
	}

	if err := PatchUserDeviceBindings(db, "main", "user1", []string{"dev1", "dev2"}, nil); err != nil {
		t.Fatal(err)
	}
	got, err = LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	u = got.Users["user1"]
	if u.MaxDevices != 8 {
		t.Fatalf("max_devices = %d, want 8 after second bind", u.MaxDevices)
	}
	if len(u.DeviceIDs) != 2 {
		t.Fatalf("device_ids len = %d, want 2", len(u.DeviceIDs))
	}
}
