package main

// keys.go: API keys.
//
// Keys are read from environment variables. To make life easier for people who
// don't use a terminal, keys typed in the UI are saved in a local ".env" file
// (readable only by the current user) and loaded on startup. bancada.yaml only
// ever contains the NAME of the variable, never the key itself.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const envFile = ".env"

// loadEnvFile copies KEY=VALUE lines from .env into the process environment.
// Variables that are already set in the real environment win.
func loadEnvFile() {
	values, err := readEnvFile()
	if err != nil {
		return
	}
	for k, v := range values {
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}

func readEnvFile() (map[string]string, error) {
	f, err := os.Open(envFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return values, scanner.Err()
}

// saveKey stores one key in .env and in the running process.
func saveKey(envName, value string) error {
	if !validEnvName(envName) {
		return fmt.Errorf(tr("nome de variável inválido: %q", "invalid variable name: %q"), envName)
	}
	if strings.ContainsAny(value, "\r\n") {
		return errors.New(tr("a chave não pode ter quebras de linha", "the key cannot contain line breaks"))
	}
	values, _ := readEnvFile()
	if values == nil {
		values = map[string]string{}
	}
	values[envName] = value

	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Bancada API keys / chaves de API. Do not share this file / não compartilhe este arquivo.\n")
	for _, k := range names {
		fmt.Fprintf(&b, "%s=%s\n", k, values[k])
	}
	if err := writeFileAtomic(envFile, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Setenv(envName, value)
}

// apiKey returns the key for a provider ("" if none is configured).
func apiKey(p ProviderConfig) string {
	if p.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(p.APIKeyEnv)
}

// maskKey shows only the last 4 characters: "••••abcd".
func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		isLetter := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !(isDigit && i > 0) {
			return false
		}
	}
	return true
}
