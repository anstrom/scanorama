// Package discovery_test verifies that the "gone" host-status model additions
// are correctly defined and accessible from outside the db package.
package discovery_test

import (
	"testing"

	"github.com/anstrom/scanorama/internal/db"
)

// TestHostStatusGoneConstant verifies the new "gone" status value is correct
// and distinct from all existing status constants.
func TestHostStatusGoneConstant(t *testing.T) {
	t.Parallel()

	if db.HostStatusGone != "gone" {
		t.Errorf("HostStatusGone = %q, want %q", db.HostStatusGone, "gone")
	}

	statuses := []string{db.HostStatusUp, db.HostStatusDown, db.HostStatusUnknown}
	for _, s := range statuses {
		if s == db.HostStatusGone {
			t.Errorf("HostStatusGone collides with existing status constant %q", s)
		}
	}
}

// TestHostStatusEventFields verifies the HostStatusEvent struct has the
// expected exported fields so API serialization won't silently omit them.
func TestHostStatusEventFields(t *testing.T) {
	t.Parallel()

	evt := db.HostStatusEvent{
		FromStatus: "up",
		ToStatus:   "gone",
	}

	if evt.FromStatus != "up" {
		t.Errorf("FromStatus = %q, want %q", evt.FromStatus, "up")
	}
	if evt.ToStatus != "gone" {
		t.Errorf("ToStatus = %q, want %q", evt.ToStatus, "gone")
	}
}
