package cmd

import (
	"strings"
	"testing"
)

func TestEnvOrDefault_Set(t *testing.T) {
	t.Setenv("TEST_ENVORD_KEY", "myval")
	got := envOrDefault("TEST_ENVORD_KEY", "fallback")
	if got != "myval" {
		t.Errorf("envOrDefault = %q, want %q", got, "myval")
	}
}

func TestEnvOrDefault_Unset(t *testing.T) {
	t.Setenv("TEST_ENVORD_KEY", "")
	got := envOrDefault("TEST_ENVORD_KEY", "fallback")
	if got != "fallback" {
		t.Errorf("envOrDefault = %q, want %q", got, "fallback")
	}
}

func TestEnvOrDefault_Empty(t *testing.T) {
	t.Setenv("TEST_ENVORD_KEY", "")
	got := envOrDefault("TEST_ENVORD_KEY", "default")
	if got != "default" {
		t.Errorf("envOrDefault with empty string = %q, want %q", got, "default")
	}
}

func TestServeCmd_MissingHost(t *testing.T) {
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")
	t.Setenv("TRUENAS_READ_ONLY", "")

	cmd := NewServeCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing host")
	}
	if !strings.Contains(err.Error(), "host is required") {
		t.Errorf("error = %q, want it to contain 'host is required'", err.Error())
	}
}

func TestServeCmd_MissingAPIKey(t *testing.T) {
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")
	t.Setenv("TRUENAS_READ_ONLY", "")

	cmd := NewServeCmd()
	cmd.SetArgs([]string{"--host", "fake.local"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !strings.Contains(err.Error(), "API key is required") {
		t.Errorf("error = %q, want it to contain 'API key is required'", err.Error())
	}
}

func TestServeCmd_ReadOnlyEnvQuirk(t *testing.T) {
	// Any non-empty value of TRUENAS_READ_ONLY enables read-only mode,
	// including "false" or "0".
	t.Setenv("TRUENAS_READ_ONLY", "false")
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")

	cmd := NewServeCmd()
	readOnly, err := cmd.Flags().GetBool("read-only")
	if err != nil {
		t.Fatalf("getting read-only flag: %v", err)
	}
	if !readOnly {
		t.Error("TRUENAS_READ_ONLY='false' should still enable read-only mode (any non-empty value)")
	}
}
