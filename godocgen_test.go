package main

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
)

func TestParseConfigWithoutEnvOverwrite(t *testing.T) {
	mockUser := "someGitUser"
	mockPass := "someGitPass"

	// Test preparation
	mockConfValues := map[string]string{
		"user": mockUser,
		"pass": mockPass,
	}
	mockConfigFile, err := createMockConfig("mock_config", mockConfValues)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	defer cleanupConfigFile(mockConfigFile)

	// Test body
	config, err := parseConfig(options{config: mockConfigFile.Name()})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if config.User != mockUser {
		t.Errorf("expected config.User to be equal to config user")
	}

	if config.Pass != mockPass {
		t.Errorf("expected config.Pass to be equal to config pass")
	}
}

func TestParseConfigWithEnvOverwrite(t *testing.T) {
	mockUser := "someGitUser"
	mockPass := "someGitPass"

	mockEnvUser := "someEnvGitUser"
	mockEnvPass := "someEnvGitPass"

	// Test preparation
	mockConfValues := map[string]string{
		"user": mockUser,
		"pass": mockPass,
	}
	mockConfigFile, err := createMockConfig("mock_config", mockConfValues)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	defer cleanupConfigFile(mockConfigFile)

	err = os.Setenv("GITHUB_USER", mockEnvUser)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = os.Setenv("GITHUB_PASS", mockEnvPass)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	defer cleanupEnvVars()

	// Test body
	config, err := parseConfig(options{config: mockConfigFile.Name()})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if config.User != mockEnvUser {
		t.Errorf("expected config.User to be equal to env user")
	}

	if config.Pass != mockEnvPass {
		t.Errorf("expected config.Pass to be equal to env pass")
	}
}

func TestParseConfigNoUserError(t *testing.T) {
	// Test preparation
	mockConfValues := map[string]string{}
	mockConfigFile, err := createMockConfig("mock_config", mockConfValues)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	defer cleanupConfigFile(mockConfigFile)

	// Test body
	_, err = parseConfig(options{config: mockConfigFile.Name()})
	if err != errNoUser {
		t.Errorf("expected errNoUser, got %v", err)
	}
}

func TestParseConfigNoPassError(t *testing.T) {
	// Test preparation
	mockConfValues := map[string]string{
		"user": "someUser",
	}
	mockConfigFile, err := createMockConfig("mock_config", mockConfValues)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	defer cleanupConfigFile(mockConfigFile)

	// Test body
	_, err = parseConfig(options{config: mockConfigFile.Name()})
	if err != errNoPass {
		t.Errorf("expected errNoPass, got %v", err)
	}
}

func TestParseConfigParseError(t *testing.T) {
	_, err := parseConfig(options{config: "invalid_file"})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// Helper function that creates a temporary config file
func createMockConfig(name string, values map[string]string) (*os.File, error) {
	file, err := ioutil.TempFile(os.TempDir(), name)
	if err != nil {
		return nil, err
	}
	conf := ""
	for key, value := range values {
		conf = conf + key + " " + value + "\n"
	}
	_, err = file.Write([]byte(conf))
	if err != nil {
		return nil, err
	}

	return file, nil
}

func cleanupConfigFile(f *os.File) {
	err := os.Remove(f.Name())
	if err != nil {
		log.Fatal(err)
	}
}

func cleanupEnvVars() {
	err := os.Setenv("GITHUB_USER", "")
	if err != nil {
		log.Fatal(err)
	}

	err = os.Setenv("GITHUB_PASS", "")
	if err != nil {
		log.Fatal(err)
	}
}
