package vm

import (
	"strings"
	"testing"
)

func TestSplitColon(t *testing.T) {
	got := splitColon("learn:x:997:997::/var/lib/learn-platform:/usr/sbin/nologin")
	want := []string{"learn", "x", "997", "997", "", "/var/lib/learn-platform", "/usr/sbin/nologin"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("splitColon mismatch.\n got=%v\nwant=%v", got, want)
	}
}

func TestLaunchJailedRejectsEmptyVMID(t *testing.T) {
	_, err := LaunchJailed(nil, JailedSpec{})
	if err == nil || !strings.Contains(err.Error(), "VMID required") {
		t.Fatalf("expected VMID-required error, got %v", err)
	}
}
