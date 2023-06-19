package types

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/simonlingoogle/go-simplelogger"
)

// GetTmpDir gets the current 'tmp' directory path
func GetTmpDir() string {
	p, _ := filepath.Abs("tmp")
	simplelogger.Debugf("GetTmpDir: %s", p)
	return "tmp"
}

// CreateTmpDir creates the tmp dir, if not already present
func CreateTmpDir() error {
	err := os.Mkdir(GetTmpDir(), 0775)
	simplelogger.Debugf("TmpDir created")
	if errors.Is(err, fs.ErrExist) {
		return nil // ok, already present
	}
	return err
}

// RemoveAllFiles removes all files specified by the glob pattern
func RemoveAllFiles(globPath string) error {
	files, err := filepath.Glob(globPath)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}
