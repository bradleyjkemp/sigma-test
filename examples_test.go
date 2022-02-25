package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExamples(t *testing.T) {
	err := filepath.Walk("./testdata/", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		t.Run(path, func(t *testing.T) {
			*fConfigFiles = "testdata/config.yaml"
			configs, err := loadConfigs()
			if err != nil {
				t.Fatal(err)
			}
			pass, err := run(path, configs, true)
			if err != nil {
				t.Fatal(err)
			}
			if !pass {
				t.Fatal("Expected all test cases to pass")
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
