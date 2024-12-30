package main

import (
	"fmt"
	"io"
	"log"
	"os"
)

type IPFSClient struct {
	Shell *shell.Shell
}

// NewIPFSClient creates a new instance of IPFSClient.
func NewIPFSClient(apiURL string) *IPFSClient {
	return &IPFSClient{
		Shell: shell.NewShell(apiURL),
	}
}

// AddFile uploads a file to IPFS.
func (client *IPFSClient) AddFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	hash, err := client.Shell.Add(file)
	if err != nil {
		return "", fmt.Errorf("failed to add file to IPFS: %v", err)
	}

	return hash, nil
}

// GetFile downloads a file from IPFS and saves it to the specified path.
func (client *IPFSClient) GetFile(hash string, outputPath string) error {
	reader, err := client.Shell.Cat(hash)
	if err != nil {
		return fmt.Errorf("failed to fetch file from IPFS: %v", err)
	}
	defer reader.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

// PinFile pins a file on IPFS.
func (client *IPFSClient) PinFile(hash string) error {
	if err := client.Shell.Pin(hash); err != nil {
		return fmt.Errorf("failed to pin file: %v", err)
	}
	return nil
}

// UnpinFile unpins a file from IPFS.
func (client *IPFSClient) UnpinFile(hash string) error {
	if err := client.Shell.Unpin(hash); err != nil {
		return fmt.Errorf("failed to unpin file: %v", err)
	}
	return nil
}

// Example usage
func main() {
	apiURL := "localhost:5001"
	client := NewIPFSClient(apiURL)

	// Add a file to IPFS
	hash, err := client.AddFile("example.txt")
	if err != nil {
		log.Fatalf("Error adding file: %v", err)
	}
	log.Printf("File added to IPFS with hash: %s", hash)

	// Download the file from IPFS
	if err := client.GetFile(hash, "downloaded_example.txt"); err != nil {
		log.Fatalf("Error getting file: %v", err)
	}
	log.Println("File downloaded successfully")

	// Pin the file
	if err := client.PinFile(hash); err != nil {
		log.Fatalf("Error pinning file: %v", err)
	}
	log.Println("File pinned successfully")

	// Unpin the file
	if err := client.UnpinFile(hash); err != nil {
		log.Fatalf("Error unpinning file: %v", err)
	}
	log.Println("File unpinned successfully")
}
