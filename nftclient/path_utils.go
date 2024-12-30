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

func createDirIfNotExist(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	}
	return nil
}

func getUserDataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./data"
	}
	return filepath.Join(homeDir, "NFTClientData")
}
