package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("Starting NFT Client...")

	// Inițializează UI-ul
	err := initializeUI()
	if err != nil {
		log.Fatalf("Error initializing UI: %v", err)
	}

	// Montează drive-ul virtual
	err = mountVirtualDrive("NFTDrive", "C:\\NFTMountPoint")
	if err != nil {
		log.Fatalf("Error mounting virtual drive: %v", err)
	}

	// Conectează la IPFS
	err = connectToIPFS()
	if err != nil {
		log.Fatalf("Error connecting to IPFS: %v", err)
	}

	fmt.Println("NFT Client started successfully!")
}
