package main

import "testing"

// The whole point of this setting: an existing family that has never touched
// it gets Fast Mode ON for Child Mode without doing anything, because a
// pointer's zero value (nil) must read as "on", not "off".
func TestChildFastModeDefaultsOn(t *testing.T) {
	if !(familyState{}).childFastMode() {
		t.Fatal("an untouched family should default to child fast mode ON")
	}
	off := false
	if (familyState{ChildFastMode: &off}).childFastMode() {
		t.Fatal("explicit off should stay off")
	}
	on := true
	if !(familyState{ChildFastMode: &on}).childFastMode() {
		t.Fatal("explicit on should stay on")
	}
}
