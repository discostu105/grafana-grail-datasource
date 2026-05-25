package plugin

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDataObjectsCache_MissThenHit(t *testing.T) {
	var c dataObjectsCache
	if _, ok := c.get(); ok {
		t.Fatal("empty cache should miss")
	}
	c.set([]DataObject{{Name: "logs", DisplayName: "Logs"}})
	got, ok := c.get()
	if !ok || len(got) != 1 || got[0].Name != "logs" {
		t.Fatalf("cache hit failed: ok=%v got=%v", ok, got)
	}
}

func TestDataObjectsCache_TTLExpiry(t *testing.T) {
	var c dataObjectsCache
	c.set([]DataObject{{Name: "spans", DisplayName: "Spans"}})
	// Simulate an expired entry by rewinding `at` past the TTL.
	c.mu.Lock()
	c.at = time.Now().Add(-2 * dataObjectsTTL)
	c.mu.Unlock()
	if _, ok := c.get(); ok {
		t.Fatal("expired cache entry should miss")
	}
}

func TestMarshalDataObjects(t *testing.T) {
	got := marshalDataObjects([]DataObject{
		{Name: "logs", DisplayName: "Logs"},
		{Name: "spans", DisplayName: "Spans"},
	})
	var roundTrip []DataObject
	if err := json.Unmarshal(got, &roundTrip); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(roundTrip) != 2 || roundTrip[0].Name != "logs" || roundTrip[1].DisplayName != "Spans" {
		t.Fatalf("unexpected payload: %+v", roundTrip)
	}
}
