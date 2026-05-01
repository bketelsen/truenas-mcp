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
	t.Setenv("TRUENAS_ENABLE_WRITES", "")
	t.Setenv("TRUENAS_TLS_INSECURE", "")

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
	t.Setenv("TRUENAS_ENABLE_WRITES", "")
	t.Setenv("TRUENAS_TLS_INSECURE", "")

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

func TestServeCmd_WritesDisabledByDefault(t *testing.T) {
	t.Setenv("TRUENAS_ENABLE_WRITES", "")
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")

	cmd := NewServeCmd()
	enableWrites, err := cmd.Flags().GetBool("enable-writes")
	if err != nil {
		t.Fatalf("getting enable-writes flag: %v", err)
	}
	if enableWrites {
		t.Error("writes should be disabled by default")
	}
}

func TestServeCmd_EnableWritesEnv(t *testing.T) {
	t.Setenv("TRUENAS_ENABLE_WRITES", "true")
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")

	cmd := NewServeCmd()
	enableWrites, err := cmd.Flags().GetBool("enable-writes")
	if err != nil {
		t.Fatalf("getting enable-writes flag: %v", err)
	}
	if !enableWrites {
		t.Error("TRUENAS_ENABLE_WRITES=true should enable writes")
	}
}

func TestServeCmd_EnableWritesFalseEnv(t *testing.T) {
	t.Setenv("TRUENAS_ENABLE_WRITES", "false")
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")

	cmd := NewServeCmd()
	enableWrites, err := cmd.Flags().GetBool("enable-writes")
	if err != nil {
		t.Fatalf("getting enable-writes flag: %v", err)
	}
	if enableWrites {
		t.Error("TRUENAS_ENABLE_WRITES=false should keep writes disabled")
	}
}

func TestServeCmd_TLSInsecureEnv(t *testing.T) {
	t.Setenv("TRUENAS_TLS_INSECURE", "1")
	t.Setenv("TRUENAS_HOST", "")
	t.Setenv("TRUENAS_API_KEY", "")

	cmd := NewServeCmd()
	tlsInsecure, err := cmd.Flags().GetBool("tls-insecure")
	if err != nil {
		t.Fatalf("getting tls-insecure flag: %v", err)
	}
	if !tlsInsecure {
		t.Error("TRUENAS_TLS_INSECURE=1 should skip TLS verification")
	}
}
