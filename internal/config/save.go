package config

import (
	"encoding/json"
	"os"
)

// Save writes the configuration back to the file
func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
