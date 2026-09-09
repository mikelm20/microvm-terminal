package main

import "testing"

func TestNumField(t *testing.T) {
	m := map[string]any{"cols": float64(120), "rows": 40}
	if numField(m, "cols") != 120 || numField(m, "rows") != 40 || numField(m, "missing") != 0 {
		t.Fatalf("numField mismatch: %v", m)
	}
}

func TestApplyWinsizeRejectsZero(t *testing.T) {
	if err := applyWinsize(0, 24); err == nil {
		t.Fatal("zero cols accepted")
	}
}
