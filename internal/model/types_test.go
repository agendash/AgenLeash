package model

import "testing"

func TestFeatureSetClone(t *testing.T) {
	original := FeatureSet{"planMode": true}
	cloned := original.Clone()

	cloned["planMode"] = false

	if original["planMode"] != true {
		t.Fatalf("clone mutated original map")
	}
}
