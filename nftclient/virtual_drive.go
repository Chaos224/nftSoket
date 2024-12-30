package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Montarea discului virtual
func mountVirtualDrive(driveLetter, mountPoint string) error {
	cmd := exec.Command("net", "use", driveLetter, mountPoint)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func unmountVirtualDrive(driveLetter string) error {
	cmd := exec.Command("net", "use", driveLetter, "/delete")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Sincronizarea fișierelor
func startSyncService(mountPoint, userDir string) error {
	fmt.Println("[INFO] Starting synchronization service...")

	ticker := time.NewTicker(10 * time.Second) // Sincronizare la fiecare 10 secunde
	defer ticker.Stop()

	for range ticker.C {
		fmt.Println("[INFO] Checking for changes in mounted directory...")
		if err := syncChanges(mountPoint, userDir); err != nil {
			fmt.Printf("[ERROR] Synchronization failed: %v\n", err)
		}
	}

	return nil
}

// Detectarea modificărilor și sincronizarea fișierelor
func syncChanges(mountPoint, userDir string) error {
	fmt.Printf("[INFO] Scanning directory for changes: %s\n", mountPoint)

	// Parcugerea directorului montat pentru fișiere noi sau modificate
	return filepath.Walk(mountPoint, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("[ERROR] Failed to access path %s: %w", path, err)
		}

		// Dacă găsim un fișier nou sau modificat, îl sincronizăm
		if !info.IsDir() {
			fmt.Printf("[INFO] Syncing file: %s\n", path)
			if err := uploadFileToServer(path); err != nil {
				return fmt.Errorf("[ERROR] Failed to sync file %s: %w", path, err)
			}
		}
		return nil
	})
}

// Încărcarea fișierului pe server (Placeholder)
func uploadFileToServer(filePath string) error {
	fmt.Printf("[INFO] Uploading file to server: %s\n", filePath)

	// Aici poți adăuga logica reală pentru încărcare pe server, utilizând un API
	return nil
}

func main() {
	driveLetter := "Z:"
	mountPoint := "\\\\server\\share"

	if err := mountVirtualDrive(driveLetter, mountPoint); err != nil {
		fmt.Println("Failed to mount virtual drive:", err)
		return
	}
	defer unmountVirtualDrive(driveLetter)

	fmt.Println("Virtual drive mounted successfully.")
}
