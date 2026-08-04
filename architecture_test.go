package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyFLTKBindingIsAbsent(t *testing.T) {
	forbidden := "github.com/george012/" + "fltk_go"
	root := "."
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "tmp" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".mod" && ext != ".sum" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(forbidden)) {
			t.Errorf("legacy FLTK binding found in %s", strings.TrimPrefix(path, root+string(filepath.Separator)))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository for legacy FLTK binding: %v", err)
	}
}
