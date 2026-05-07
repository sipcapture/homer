// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package homerconfig

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/mcuadros/go-defaults"
	"github.com/spf13/viper"
)

const (
	jsonExt = ".json"
)

var (
	// TLS version mapping for efficient lookup
	tlsVersionMap = map[string]uint16{
		"TLS1.0": tls.VersionTLS10,
		"TLS1.1": tls.VersionTLS11,
		"TLS1.2": tls.VersionTLS12,
		"TLS1.3": tls.VersionTLS13,
	}
	defaultTLSVersion uint16 = tls.VersionTLS12
)

// HomerServerConfig manages server configuration
type HomerServerConfig struct {
	configPaths []string
	logName     string
	logPath     string
	Setting     *HomerServerSettings
	Internal    *InternalSettings // Internal runtime parameters (not part of config)
	viper       *viper.Viper      // Use local viper instance instead of global
}

// HomerCoreType represents application type (reserved / legacy layout).
type HomerCoreType int

const (
	HomerCoreWriter HomerCoreType = iota
)

// New creates a new HomerServerConfig instance
func New(configPaths []string, logName, logPath string) *HomerServerConfig {
	c := &HomerServerConfig{
		configPaths: configPaths,
		logName:     logName,
		logPath:     logPath,
		Setting:     new(HomerServerSettings),
		Internal:    new(InternalSettings),
		viper:       viper.New(), // Create local viper instance
	}

	// Set default values for configuration
	defaults.SetDefaults(c.Setting)

	// Initialize internal settings with defaults
	c.Internal.EnvPrefix = "HOMERCORE"
	c.Internal.FingerPrintType = 1
	c.Internal.DataBaseStrategy = 0
	c.Internal.CurrentDataBaseIndex = 0
	c.Internal.DataDatabaseGroupNodeMap = make(map[string][]string)
	c.Internal.CloudCaptureCategory = []string{"settings", "statistics", "active", "regexp", "correlation", "statsgroup", "mos"}

	return c
}

// readConfigFromFS reads configuration files from filesystem
func (c *HomerServerConfig) readConfigFromFS() error {
	var firstFile bool = true

	for _, path := range c.configPaths {
		// Validate file extension
		if !strings.HasSuffix(path, jsonExt) {
			return fmt.Errorf("only %s extension allowed: %s", jsonExt, path)
		}

		// Check if file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // Skip non-existent files
		}

		// Set config file
		c.viper.SetConfigFile(path)

		// Read first file or merge subsequent files
		if firstFile {
			if err := c.viper.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read config file %s: %w", path, err)
			}
			firstFile = false
		} else {
			if err := c.viper.MergeInConfig(); err != nil {
				return fmt.Errorf("failed to merge config file %s: %w", path, err)
			}
		}
	}

	if firstFile {
		return errors.New("no valid configuration files found")
	}

	return nil
}

// ReadConfig reads and parses configuration from files and environment
func (c *HomerServerConfig) ReadConfig() error {
	// Read configuration files
	if err := c.readConfigFromFS(); err != nil {
		return fmt.Errorf("failed to read config from filesystem: %w", err)
	}

	// Setup environment variable binding
	c.setupEnvBinding()

	// Unmarshal configuration
	if err := c.unmarshalConfig(); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply command line overrides
	c.applyCommandLineOverrides()

	// Process TLS version strings
	c.processTLSVersions()

	return nil
}

// setupEnvBinding configures environment variable binding
func (c *HomerServerConfig) setupEnvBinding() {
	c.viper.AutomaticEnv()
	c.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "[", "_", "]", ""))
	c.viper.SetEnvPrefix(c.Internal.EnvPrefix)
	c.bindEnvs(c.Setting) // Pass pointer to avoid copying mutex
}

// unmarshalConfig unmarshals viper config into Setting struct
func (c *HomerServerConfig) unmarshalConfig() error {
	err := c.viper.Unmarshal(c.Setting, viper.DecoderConfigOption(func(config *mapstructure.DecoderConfig) {
		config.TagName = "json"
		config.ErrorUnused = false // Don't error on unused fields
		config.WeaklyTypedInput = true
	}))

	if err != nil {
		// Ignore specific errors for backward compatibility
		if strings.Contains(err.Error(), "database_config[host]") {
			return nil // Old config format, ignore error
		}
		return err
	}

	return nil
}

// applyCommandLineOverrides applies command line parameter overrides
func (c *HomerServerConfig) applyCommandLineOverrides() {
	if c.logName != "" {
		c.Setting.LOG_SETTINGS.Name = c.logName
	}
	if c.logPath != "" {
		c.Setting.LOG_SETTINGS.Path = c.logPath
	}
}

// processTLSVersions converts TLS version strings to uint16 constants
func (c *HomerServerConfig) processTLSVersions() {
	// Process HTTPS server TLS versions
	c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MinTLSVersionNum = parseTLSVersion(c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MinTLSVersion)
	c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MaxTLSVersionNum = parseTLSVersion(c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MaxTLSVersion)

	// Also process old location for backward compatibility
	c.Setting.HTTPS_SETTINGS.MinTLSVersion = parseTLSVersion(c.Setting.HTTPS_SETTINGS.MinTLSVersionString)
	c.Setting.HTTPS_SETTINGS.MaxTLSVersion = parseTLSVersion(c.Setting.HTTPS_SETTINGS.MaxTLSVersionString)

	// Note: TCP server TLS versions are stored as strings in the struct,
	// they will be parsed when TLS server is initialized
}

// parseTLSVersion converts TLS version string to tls.Version constant
func parseTLSVersion(version string) uint16 {
	if v, ok := tlsVersionMap[version]; ok {
		return v
	}
	return defaultTLSVersion
}

// bindEnvs recursively binds environment variables to configuration struct
func (c *HomerServerConfig) bindEnvs(iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)

	// Handle pointer types
	if ift.Kind() == reflect.Ptr {
		if ifv.IsNil() {
			return
		}
		ifv = ifv.Elem()
		ift = ift.Elem()
	}

	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)

		// Skip unexported fields
		if !v.CanSet() {
			continue
		}

		// Get mapstructure tag
		tv, ok := t.Tag.Lookup("mapstructure")
		if !ok {
			continue
		}

		// Handle different field types
		switch v.Kind() {
		case reflect.Map, reflect.Slice:
			// Skip maps and slices for now
			continue
		case reflect.Struct:
			// Recursively bind nested structs
			c.bindEnvs(v.Interface(), append(parts, tv)...)
		default:
			// Bind environment variable
			envKey := strings.Join(append(parts, tv), ".")
			if err := c.viper.BindEnv(envKey); err != nil {
				// Log but don't fail on bind errors
				fmt.Printf("warning: couldn't bind env variable %s: %v\n", envKey, err)
			}
		}
	}
}

// SafeConfig saves configuration to file after removing sensitive variables
func (c *HomerServerConfig) SafeConfig() error {
	cfg := c.viper.AllSettings()

	// List of variables to remove (can be extended)
	varsToRemove := []string{
		// Add sensitive variables here if needed
	}

	// Remove sensitive variables
	for _, varPath := range varsToRemove {
		if err := removeNestedKey(cfg, strings.Split(varPath, ".")); err != nil {
			return fmt.Errorf("failed to remove key %s: %w", varPath, err)
		}
	}

	// Marshal to JSON
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Read back into viper
	if err := c.viper.ReadConfig(bytes.NewReader(b)); err != nil {
		return fmt.Errorf("failed to read config from buffer: %w", err)
	}

	// Write to config file
	return c.viper.WriteConfig()
}

// removeNestedKey removes a nested key from a map
func removeNestedKey(m map[string]interface{}, parts []string) error {
	if len(parts) == 0 {
		return nil
	}

	key := parts[0]
	val, ok := m[key]
	if !ok {
		return nil // Key doesn't exist, nothing to remove
	}

	if len(parts) == 1 {
		// Last part, delete the key
		delete(m, key)
		return nil
	}

	// Navigate deeper
	subMap, ok := val.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unsupported type: %T for %q", val, key)
	}

	return removeNestedKey(subMap, parts[1:])
}

// Validate validates the configuration
func (c *HomerServerConfig) Validate() error {
	// Validate HTTPS server
	if c.Setting.SERVER_SETTINGS.HTTPS_SERVER.Enable {
		if c.Setting.SERVER_SETTINGS.HTTPS_SERVER.Cert == "" || c.Setting.SERVER_SETTINGS.HTTPS_SERVER.Key == "" {
			return errors.New("HTTPS server enabled but certificate or key not provided")
		}
	}

	// Validate TLS versions for HTTPS server
	if c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MinTLSVersionNum > c.Setting.SERVER_SETTINGS.HTTPS_SERVER.MaxTLSVersionNum {
		return errors.New("HTTPS server min TLS version is greater than max version")
	}

	// Validate TLS server
	if c.Setting.SERVER_SETTINGS.TLS_SERVER.Enable {
		if c.Setting.SERVER_SETTINGS.TLS_SERVER.Cert == "" || c.Setting.SERVER_SETTINGS.TLS_SERVER.Key == "" {
			return errors.New("TLS server enabled but certificate or key not provided")
		}
	}

	// Also validate old location for backward compatibility
	if c.Setting.HTTPS_SETTINGS.Enable {
		if c.Setting.HTTPS_SETTINGS.Cert == "" || c.Setting.HTTPS_SETTINGS.Key == "" {
			return errors.New("HTTPS enabled but certificate or key not provided")
		}
	}

	// Validate TLS version ranges for old location
	if c.Setting.HTTPS_SETTINGS.MinTLSVersion > c.Setting.HTTPS_SETTINGS.MaxTLSVersion {
		return errors.New("HTTPS min TLS version is greater than max version")
	}

	return nil
}

// GetConfigPaths returns the list of configuration file paths
func (c *HomerServerConfig) GetConfigPaths() []string {
	return c.configPaths
}

// Reload reloads configuration from files
func (c *HomerServerConfig) Reload() error {
	// Create new viper instance
	c.viper = viper.New()

	// Re-read configuration
	return c.ReadConfig()
}

// GenerateExampleConfig generates an example JSON configuration with default values
func GenerateExampleConfig() (string, error) {
	// Create a new settings instance with defaults
	settings := new(HomerServerSettings)
	defaults.SetDefaults(settings)

	// Create a map to filter out internal/private fields
	configMap := make(map[string]interface{})

	// Convert struct to map using reflection, filtering out internal fields
	settingsValue := reflect.ValueOf(settings).Elem()
	settingsType := settingsValue.Type()

	for i := 0; i < settingsValue.NumField(); i++ {
		field := settingsType.Field(i)
		fieldValue := settingsValue.Field(i)

		// Skip unexported fields and internal fields
		if !fieldValue.CanInterface() {
			continue
		}

		// Skip internal fields that shouldn't be in config
		// Note: Internal fields (SrartTime, DataDatabaseGroupNodeMap, Validate, etc.)
		// are now in InternalSettings struct, not in HomerServerSettings
		fieldName := field.Name
		if fieldName == "MinTLSVersion" || fieldName == "MaxTLSVersion" {
			continue
		}

		// Get JSON tag name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Extract JSON tag name (before comma if exists)
		jsonName := strings.Split(jsonTag, ",")[0]
		if jsonName == "" {
			jsonName = fieldName
		}

		// Handle nested structs
		if fieldValue.Kind() == reflect.Struct {
			nestedMap := make(map[string]interface{})
			nestedType := fieldValue.Type()

			for j := 0; j < fieldValue.NumField(); j++ {
				nestedField := nestedType.Field(j)
				nestedValue := fieldValue.Field(j)

				if !nestedValue.CanInterface() {
					continue
				}

				// Skip internal fields
				if nestedField.Name == "MinTLSVersion" || nestedField.Name == "MaxTLSVersion" {
					continue
				}

				nestedJSONTag := nestedField.Tag.Get("json")
				if nestedJSONTag == "" || nestedJSONTag == "-" {
					continue
				}

				nestedJSONName := strings.Split(nestedJSONTag, ",")[0]
				if nestedJSONName == "" {
					nestedJSONName = nestedField.Name
				}

				// Convert value to interface
				if nestedValue.Kind() == reflect.Ptr {
					if nestedValue.IsNil() {
						continue
					}
					nestedMap[nestedJSONName] = nestedValue.Elem().Interface()
				} else {
					nestedMap[nestedJSONName] = nestedValue.Interface()
				}
			}

			if len(nestedMap) > 0 {
				configMap[jsonName] = nestedMap
			}
		} else {
			// Convert value to interface
			if fieldValue.Kind() == reflect.Ptr {
				if fieldValue.IsNil() {
					continue
				}
				configMap[jsonName] = fieldValue.Elem().Interface()
			} else {
				configMap[jsonName] = fieldValue.Interface()
			}
		}
	}

	// Marshal to JSON with indentation
	jsonBytes, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal example config: %w", err)
	}

	return string(jsonBytes), nil
}
