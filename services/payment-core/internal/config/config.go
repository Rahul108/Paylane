package config

import (
	"os"
)

type Config struct {
	ServiceName string
	Port        string
	KeysDir     string
	DatabaseURL string
}

func Load(serviceName, defaultPort string) *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	keysDir := os.Getenv("KEYS_DIR")
	if keysDir == "" {
		// Try standard paths
		if _, err := os.Stat("/keys"); err == nil {
			keysDir = "/keys"
		} else if _, err := os.Stat("../../keys"); err == nil {
			keysDir = "../../keys"
		} else if _, err := os.Stat("keys"); err == nil {
			keysDir = "keys"
		} else {
			keysDir = "./keys"
		}
	}

	dbURL := os.Getenv("DATABASE_URL")

	return &Config{
		ServiceName: serviceName,
		Port:        port,
		KeysDir:     keysDir,
		DatabaseURL: dbURL,
	}
}
