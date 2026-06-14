package paneldb

import (
	"testing"
)

func TestUpdateUsersTraffic(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	s := NewStore()
	s.MainPassword = "main"
	s.Users["u1"] = &User{UpBytes: 100, DownBytes: 200}
	if err := SaveStore(db, s, SaveOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := UpdateUsersTraffic(db, map[string]TrafficSnapshot{
		"u1": {UpBytes: 500, DownBytes: 800, LastSeenAt: 12345},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadStore(db)
	if err != nil {
		t.Fatal(err)
	}
	u := got.Users["u1"]
	if u == nil || u.UpBytes != 500 || u.DownBytes != 800 || u.LastSeenAt != 12345 {
		t.Fatalf("traffic: %+v", u)
	}
}

func TestMergeTrafficSnapshots(t *testing.T) {
	into := map[string]*User{
		"a": {UpBytes: 10, DownBytes: 20},
	}
	MergeTrafficSnapshots(into, map[string]TrafficSnapshot{
		"a": {UpBytes: 5, DownBytes: 30, LastSeenAt: 99, IsDeactivated: true},
	})
	if into["a"].UpBytes != 10 || into["a"].DownBytes != 30 || into["a"].LastSeenAt != 99 || !into["a"].IsDeactivated {
		t.Fatalf("merge: %+v", into["a"])
	}
}
