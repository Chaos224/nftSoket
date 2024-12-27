package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Dir      string `json:"dir"`
}

var (
	users      = make(map[string]User)
	usersMutex sync.Mutex
)

// Încarcă utilizatorii dintr-un fișier JSON
func loadUsers() {
	file, err := os.Open("users.json")
	if err != nil {
		log.Fatalf("Failed to open users file: %v", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&users); err != nil {
		log.Fatalf("Failed to decode users file: %v", err)
	}

	fmt.Println("Users loaded successfully.")
}

// Handler pentru autentificare
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds User
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	usersMutex.Lock()
	user, exists := users[creds.Username]
	usersMutex.Unlock()

	if !exists || user.Password != creds.Password {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	response := map[string]string{
		"message":  "Login successful",
		"user_dir": user.Dir,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Handler pentru certificatul public
func certHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("[INFO] Serving cert.pem")
	http.ServeFile(w, r, "cert.pem")
}

func main() {
	loadUsers()

	// Configurare Handlers
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/cert", certHandler)

	// Încarcă certificatul CA
	caCert, err := ioutil.ReadFile("cert.pem")
	if err != nil {
		log.Fatalf("Failed to load CA certificate: %v", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Configurare TLS
	tlsConfig := &tls.Config{
		ClientAuth: tls.VerifyClientCertIfGiven,
		ClientCAs:  caCertPool,
		MinVersion: tls.VersionTLS12,
	}

	server := &http.Server{
		Addr:      ":8081",
		TLSConfig: tlsConfig,
	}

	fmt.Println("Server running on https://localhost:8081")
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}
