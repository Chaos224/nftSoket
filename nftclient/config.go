package main

import (
	"encoding/json"
	"os"
)

const configFile = "config.json"

type Config struct {
	ServerAddress string `json:"server_address"`
}

func getServerConfig() string {
	file, err := os.Open(configFile)
	if err != nil {
		return "https://localhost:8081"
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return "https://localhost:8081"
	}
	return config.ServerAddress
}

func saveServerConfig(address string) {
	config := Config{ServerAddress: address}
	file, err := os.Create(configFile)
	if err != nil {
		panic("Failed to save config")
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(&config)
}
