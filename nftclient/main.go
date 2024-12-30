package main

import (
	"fmt"
	"log"

	"fyne.io/fyne/v2/app"
)

func main() {
	// Inițializare aplicație
	a := app.New()
	w := createMainWindow(a)

	// Log pentru pornire
	log.Println("NFTClient started...")

	server := getServerConfig()
	fmt.Println("Connecting to server:", server)

	// Example login
	message, userDir, ipfsHash := login("username", "password")
	fmt.Println("Message:", message)
	fmt.Println("User Directory:", userDir)
	fmt.Println("IPFS Hash:", ipfsHash)

	// Mount virtual drive
	driveLetter := "Z:"
	mountPoint := userDir
	if err := mountVirtualDrive(driveLetter, mountPoint); err != nil {
		fmt.Println("Failed to mount virtual drive:", err)
		return
	}
	defer unmountVirtualDrive(driveLetter)

	// Afișare fereastră principală
	w.ShowAndRun()
}
