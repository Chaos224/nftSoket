package main

import (
	"os"
	"path/filepath"
)

func GetExecutableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(exePath)
}
