package main

import "testing"

func TestExamples(t *testing.T) {
	pass, err := run("./testdata", true)
	if err != nil {
		t.Fatal(err)
	}
	if !pass {
		t.Fatal("Expected all test cases to pass")
	}
}
