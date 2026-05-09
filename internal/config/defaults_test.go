package config

import (
	"encoding/json"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestGenerateInitialConfigUsesFreshCredentials(t *testing.T) {
	t.Parallel()

	first, err := GenerateInitialConfig()
	if err != nil {
		t.Fatalf("generate first config: %v", err)
	}
	second, err := GenerateInitialConfig()
	if err != nil {
		t.Fatalf("generate second config: %v", err)
	}

	var firstCfg Config
	if err := json.Unmarshal([]byte(first.Content), &firstCfg); err != nil {
		t.Fatalf("parse first config: %v", err)
	}
	var secondCfg Config
	if err := json.Unmarshal([]byte(second.Content), &secondCfg); err != nil {
		t.Fatalf("parse second config: %v", err)
	}

	if firstCfg.SessionKey == "" || firstCfg.SessionKey == "change-me" {
		t.Fatalf("session key was not randomized: %q", firstCfg.SessionKey)
	}
	if secondCfg.SessionKey == firstCfg.SessionKey {
		t.Fatalf("session keys should differ")
	}
	if first.Password == "" || first.Password == second.Password {
		t.Fatalf("initial passwords should be non-empty and unique")
	}
	if firstCfg.Auth.PasswordHash == defaultPasswordHash {
		t.Fatalf("default password hash was not replaced")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(firstCfg.Auth.PasswordHash), []byte(first.Password)); err != nil {
		t.Fatalf("generated password does not match generated hash: %v", err)
	}
}
