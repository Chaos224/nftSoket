package main

import (
	"fmt"
	"os"
	"path/filepath"
	// Biblioteca pentru WinFsp
)

// MountVirtualDrive montează un drive virtual folosind WinFsp.
func MountVirtualDrive(mountPoint, sourcePath string) error {
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source path does not exist: %s", sourcePath)
	}

	absMountPoint, err := filepath.Abs(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to resolve mount point path: %w", err)
	}

	// Creare director dacă nu există
	if _, err := os.Stat(absMountPoint); os.IsNotExist(err) {
		if mkdirErr := os.MkdirAll(absMountPoint, os.ModePerm); mkdirErr != nil {
			return fmt.Errorf("failed to create mount point directory: %w", mkdirErr)
		}
	}

	fmt.Printf("Attempting to mount source: %s at %s\n", sourcePath, absMountPoint)

	// Configurare sistem de fișiere virtual
	fs := &fuse.FSOptions{
		Name: "NFTClientDrive",
	}

	server, err := fuse.NewServer(fs, absMountPoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create FUSE server: %w", err)
	}

	// Pornire server
	go func() {
		if serveErr := server.Serve(); serveErr != nil {
			fmt.Printf("Server error: %v\n", serveErr)
		}
	}()

	// Așteptare pentru montare completă
	<-server.Ready
	if server.MountError != nil {
		return fmt.Errorf("mount failed: %w", server.MountError)
	}

	fmt.Println("Drive mounted successfully!")
	return nil
}

// UnmountVirtualDrive demontează un drive virtual.
func UnmountVirtualDrive(mountPoint string) error {
	absMountPoint, err := filepath.Abs(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to resolve mount point path: %w", err)
	}

	fmt.Printf("Attempting to unmount: %s\n", absMountPoint)
	if err := fuse.Unmount(absMountPoint); err != nil {
		return fmt.Errorf("failed to unmount drive: %w", err)
	}

	fmt.Println("Drive unmounted successfully!")
	return nil
}
