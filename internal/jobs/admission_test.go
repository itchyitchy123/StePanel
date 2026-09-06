package jobs

import "testing"

func TestAdmissionBoundsAndReleasesSlots(t *testing.T) {
	a := NewAdmission(1)
	if !a.Acquire() {
		t.Fatal("first acquire failed")
	}
	if a.Acquire() {
		t.Fatal("admission limit was not enforced")
	}
	a.Release()
	if !a.Acquire() {
		t.Fatal("released slot was not reusable")
	}
	a.Release()
}

func TestAdmissionNormalizesInvalidLimit(t *testing.T) {
	a := NewAdmission(0)
	if !a.Acquire() {
		t.Fatal("non-positive limit did not normalize")
	}
	a.Release()
}
