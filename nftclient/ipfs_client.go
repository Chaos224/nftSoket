package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type LoginResponse struct {
	Message  string `json:"message"`
	UserDir  string `json:"user_dir"`
	IPFSHash string `json:"ipfs_hash"`
}

// Descărcăm certificatul public de pe server și salvăm local
func downloadCert(certDir string) error {
	server := getServerConfig() // Adresa serverului
	certPath := filepath.Join(certDir, "server_cert.pem")

	resp, err := http.Get(server + "/cert")
	if err != nil {
		return fmt.Errorf("failed to connect to server at %s: %w", server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d when downloading certificate", resp.StatusCode)
	}

	// Creare director pentru certificate dacă nu există
	if err := os.MkdirAll(certDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Salvăm certificatul
	out, err := os.Create(certPath)
	if err != nil {
		return fmt.Errorf("failed to save certificate: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	fmt.Printf("[INFO] Certificate downloaded and saved to %s\n", certPath)
	return nil
}

// Login utilizând certificatul descărcat
func login(username, password string) (string, string, string) {
	certDir := "certs"
	certPath := filepath.Join(certDir, "server_cert.pem")

	// Descărcăm certificatul dacă nu există
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		fmt.Println("[INFO] Downloading certificate...")
		if err := downloadCert(certDir); err != nil {
			return "Failed to download certificate", "", ""
		}
	}

	// Citim certificatul descărcat
	caCert, err := os.ReadFile(certPath)
	if err != nil {
		return "Failed to read certificate", "", ""
	}

	// Adăugăm certificatul în pool-ul CA
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return "Failed to parse certificate", "", ""
	}

	// Configurăm clientul HTTP pentru a valida certificatul
	server := getServerConfig() // Adresa serverului
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool, // Adăugăm certificatul auto-semnat
			},
		},
	}

	// Trimiterea cererii de login
	data := map[string]string{"username": username, "password": password}
	body, _ := json.Marshal(data)

	resp, err := httpClient.Post(server+"/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "Connection failed", "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "Login failed", "", ""
	}

	var response LoginResponse
	json.NewDecoder(resp.Body).Decode(&response)
	return response.Message, response.UserDir, response.IPFSHash
}
