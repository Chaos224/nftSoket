package main

import (
	"encoding/json"
	"os"
	"runtime"
)

const configFile = "config.json"

type Config struct {
	ServerAddress string `json:"server_address"`
}

func getServerConfig() string {
	file, err := os.Open(configFile)
	if err != nil {
		if runtime.GOOS == "windows" {
			return "http://localhost:8081"
		}
		return "http://localhost:8081"
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		if runtime.GOOS == "windows" {
			return "http://localhost:8081"
		}
		return "http://localhost:8081"
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
