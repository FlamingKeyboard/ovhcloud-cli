// SPDX-FileCopyrightText: 2025 OVH SAS <opensource@ovh.net>
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/ini.v1"
)

var (
	ConfigPaths = DefaultConfigPaths()

	ConfigurableFields = map[string]string{
		"endpoint":              "default",
		"default_cloud_project": "ovh-cli",
	}
)

const windowsUserConfigPath = `%APPDATA%\ovhcloud\ovh.conf`

// currentUserHome attempts to get current user's home directory.
func currentUserHome() (string, error) {
	if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
		return userHome, nil
	}

	for _, envName := range []string{"HOME", "USERPROFILE"} {
		if userHome := os.Getenv(envName); userHome != "" {
			return userHome, nil
		}
	}

	if homeDrive := os.Getenv("HOMEDRIVE"); homeDrive != "" {
		if homePath := os.Getenv("HOMEPATH"); homePath != "" {
			return homeDrive + homePath, nil
		}
	}

	return "", fmt.Errorf("unable to resolve current user home directory")
}

func currentUserConfigDir() (string, error) {
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return configDir, nil
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return appData, nil
		}
	}

	return "", fmt.Errorf("unable to resolve current user config directory")
}

func appendUniquePath(paths []string, path string) []string {
	if path == "" {
		return paths
	}

	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}

	return append(paths, path)
}

func joinPathFor(goos string, elements ...string) string {
	if goos != "windows" {
		return path.Join(elements...)
	}

	cleaned := make([]string, 0, len(elements))
	for index, element := range elements {
		if element == "" {
			continue
		}

		element = strings.ReplaceAll(element, "/", `\`)
		if index == 0 {
			element = strings.TrimRight(element, `\`)
		} else {
			element = strings.Trim(element, `\`)
		}

		cleaned = append(cleaned, element)
	}

	if len(cleaned) == 0 {
		return ""
	}

	if len(cleaned) == 1 {
		return cleaned[0]
	}

	if cleaned[0] == "." {
		return strings.Join(cleaned[1:], `\`)
	}

	return strings.Join(cleaned, `\`)
}

func defaultConfigPathsFor(goos string) []string {
	switch goos {
	case "windows":
		return []string{
			windowsUserConfigPath,
			"~/.ovh.conf",
			"./ovh.conf",
		}
	default:
		return []string{
			"/etc/ovh.conf",
			"~/.ovh.conf",
			"./ovh.conf",
		}
	}
}

func defaultConfigWritePathFor(goos, userHome, userConfigDir string) string {
	switch goos {
	case "windows":
		if userConfigDir != "" {
			return joinPathFor(goos, userConfigDir, "ovhcloud", "ovh.conf")
		}
		if userHome != "" {
			return joinPathFor(goos, userHome, ".ovh.conf")
		}
	default:
		return "/etc/ovh.conf"
	}

	return joinPathFor(goos, ".", "ovh.conf")
}

func DefaultConfigPaths() []string {
	return defaultConfigPathsFor(runtime.GOOS)
}

func DefaultConfigWritePath() string {
	userHome, _ := currentUserHome()
	userConfigDir, _ := currentUserConfigDir()
	return defaultConfigWritePathFor(runtime.GOOS, userHome, userConfigDir)
}

func prepareConfigWritePath(path string) (string, error) {
	if path == "" {
		path = DefaultConfigWritePath()
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	return path, nil
}

func expandConfigPath(configPath, goos, userHome, userConfigDir string) string {
	switch {
	case configPath == windowsUserConfigPath:
		if userConfigDir == "" {
			return ""
		}

		return joinPathFor(goos, userConfigDir, "ovhcloud", "ovh.conf")
	case strings.HasPrefix(configPath, "~/"), strings.HasPrefix(configPath, `~\`):
		if userHome == "" {
			return ""
		}

		return joinPathFor(goos, userHome, configPath[2:])
	case strings.HasPrefix(configPath, "./"):
		return configPath[2:]
	case strings.HasPrefix(configPath, `.\\`):
		return configPath[2:]
	default:
		return configPath
	}
}

// configPaths returns configPaths, with ~/ prefix expanded.
func ExpandConfigPaths() []string {
	userHome, _ := currentUserHome()
	userConfigDir, _ := currentUserConfigDir()

	paths := make([]string, 0, len(ConfigPaths))
	for _, configPath := range ConfigPaths {
		expandedPath := expandConfigPath(configPath, runtime.GOOS, userHome, userConfigDir)
		if expandedPath == "" {
			continue
		}

		paths = append(paths, expandedPath)
	}

	return paths
}

// loadINI builds a ini.File from the configuration paths provided in configPaths.
// It's a helper for loadConfig.
func LoadINI() (*ini.File, string) {
	paths := ExpandConfigPaths()
	if len(paths) == 0 {
		return ini.Empty(), ""
	}

	for _, path := range paths {
		if cfg, err := ini.Load(path); err == nil {
			return cfg, path
		}
	}

	return ini.Empty(), ""
}

// getConfigValue returns the value of OVH_<NAME> or "name" value from "section". If
// the value could not be read from either env or any configuration files, return 'def'
func getConfigValue(cfg *ini.File, section, name, defaultValue string) string {
	// Attempt to load from environment
	fromEnv := os.Getenv("OVH_" + strings.ToUpper(name))
	if fromEnv != "" {
		return fromEnv
	}

	// Attempt to load from configuration
	fromSection := cfg.Section(section)
	if fromSection == nil {
		return defaultValue
	}

	fromSectionKey := fromSection.Key(name)
	if fromSectionKey == nil {
		return defaultValue
	}

	return fromSectionKey.String()
}

func GetConfigValue(cfg *ini.File, sectionName, keyName string) (string, error) {
	// In profile mode, read from the active profile section only (no fallback to legacy)
	if sectionName == "" {
		if profileName := GetActiveProfileName(cfg, ActiveProfileOverride); profileName != "" && !IsDefaultProfile(profileName) {
			return GetProfileConfigValue(cfg, profileName, keyName), nil
		}
	}

	if sectionName == "" {
		sectionName = ConfigurableFields[keyName]
		if sectionName == "" {
			return "", fmt.Errorf("unknown configuration field %q", keyName)
		}
	}

	return getConfigValue(cfg, sectionName, keyName, ""), nil
}

func SetConfigValue(cfg *ini.File, path, sectionName, keyName, value string) error {
	var err error
	path, err = prepareConfigWritePath(path)
	if err != nil {
		return err
	}

	if sectionName == "" {
		sectionName = ConfigurableFields[keyName]
		if sectionName == "" {
			return fmt.Errorf("unknown configuration field %q", keyName)
		}
	}

	section := cfg.Section(sectionName)
	if section == nil {
		section, err = cfg.NewSection("ovh-cli")
		if err != nil {
			return err
		}
	}

	key := section.Key(keyName)
	if section == nil {
		_, err = section.NewKey(keyName, value)
		if err != nil {
			return err
		}
	} else {
		key.SetValue(value)
	}

	return cfg.SaveTo(path)
}

// ActiveProfileOverride is set from the --profile CLI flag by the root command's
// PersistentPreRun. It allows GetConfigValue to respect the --profile flag without
// the config package needing to import the flags package.
var ActiveProfileOverride string

const profileSectionPrefix = "profile:"

// DefaultProfileName is the reserved name for the legacy/default configuration.
// It does not map to a [profile:default] section — instead it represents the
// standard go-ovh configuration from [default] + [ovh-<endpoint>] sections.
const DefaultProfileName = "default"

// IsDefaultProfile returns true if the given name is the reserved default profile.
func IsDefaultProfile(name string) bool {
	return name == DefaultProfileName
}

// GetActiveProfileName returns the active profile name by checking (in order):
// 1. The flagOverride parameter (from --profile CLI flag)
// 2. The OVH_PROFILE environment variable
// 3. The "profile" key in the [default] INI section
// Returns "" if no profile is configured (legacy mode).
func GetActiveProfileName(cfg *ini.File, flagOverride string) string {
	if flagOverride != "" {
		return flagOverride
	}
	if p := os.Getenv("OVH_PROFILE"); p != "" {
		return p
	}
	if cfg != nil {
		section := cfg.Section("default")
		if section != nil && section.HasKey("profile") {
			return section.Key("profile").String()
		}
	}
	return ""
}

// IsProfileMode returns true if a profile is active (either from flag, env, or config).
func IsProfileMode(cfg *ini.File, flagOverride string) bool {
	return GetActiveProfileName(cfg, flagOverride) != ""
}

// GetProfileSection returns the [profile:<name>] INI section for the given profile.
func GetProfileSection(cfg *ini.File, profileName string) (*ini.Section, error) {
	sectionName := profileSectionPrefix + profileName
	section := cfg.Section(sectionName)
	if section == nil || len(section.Keys()) == 0 {
		return nil, fmt.Errorf("profile %q not found in configuration", profileName)
	}
	return section, nil
}

// GetProfileCredentials reads OVHcloud API credentials from a profile section.
// Environment variables (OVH_ENDPOINT, OVH_APPLICATION_KEY, etc.) take precedence.
func GetProfileCredentials(cfg *ini.File, profileName string) (endpoint, appKey, appSecret, consumerKey string, err error) {
	section, err := GetProfileSection(cfg, profileName)
	if err != nil {
		return "", "", "", "", err
	}

	readKey := func(envName, iniKey string) string {
		if v := os.Getenv(envName); v != "" {
			return v
		}
		if section.HasKey(iniKey) {
			return section.Key(iniKey).String()
		}
		return ""
	}

	endpoint = readKey("OVH_ENDPOINT", "endpoint")
	appKey = readKey("OVH_APPLICATION_KEY", "application_key")
	appSecret = readKey("OVH_APPLICATION_SECRET", "application_secret")
	consumerKey = readKey("OVH_CONSUMER_KEY", "consumer_key")

	return endpoint, appKey, appSecret, consumerKey, nil
}

// GetProfileConfigValue reads a configuration value from a profile section.
func GetProfileConfigValue(cfg *ini.File, profileName, keyName string) string {
	section, err := GetProfileSection(cfg, profileName)
	if err != nil {
		return ""
	}
	if section.HasKey(keyName) {
		return section.Key(keyName).String()
	}
	return ""
}

// ListProfiles returns the names of all profiles found in the INI config.
func ListProfiles(cfg *ini.File) []string {
	var profiles []string
	for _, section := range cfg.Sections() {
		if name, ok := strings.CutPrefix(section.Name(), profileSectionPrefix); ok {
			profiles = append(profiles, name)
		}
	}
	return profiles
}

// SetActiveProfile sets the active profile in the [default] section.
func SetActiveProfile(cfg *ini.File, path, profileName string) error {
	var err error
	path, err = prepareConfigWritePath(path)
	if err != nil {
		return err
	}
	section := cfg.Section("default")
	if section == nil {
		section, err = cfg.NewSection("default")
		if err != nil {
			return err
		}
	}
	section.Key("profile").SetValue(profileName)
	return cfg.SaveTo(path)
}

// DeleteProfile removes a profile section from the config. If the deleted profile
// was the active one, the "profile" key is removed from [default] (falling back to legacy mode).
func DeleteProfile(cfg *ini.File, path, profileName string) error {
	var err error
	path, err = prepareConfigWritePath(path)
	if err != nil {
		return err
	}

	sectionName := profileSectionPrefix + profileName
	section := cfg.Section(sectionName)
	if section == nil || len(section.Keys()) == 0 {
		return fmt.Errorf("profile %q not found in configuration", profileName)
	}

	cfg.DeleteSection(sectionName)

	// If the deleted profile was the active one, clear the active profile
	if GetActiveProfileName(cfg, "") == profileName {
		defaultSection := cfg.Section("default")
		if defaultSection != nil {
			defaultSection.DeleteKey("profile")
		}
	}

	return cfg.SaveTo(path)
}

// SetProfileConfigValue sets a configuration value in a profile section,
// creating the section if it does not exist.
func SetProfileConfigValue(cfg *ini.File, path, profileName, keyName, value string) error {
	var err error
	path, err = prepareConfigWritePath(path)
	if err != nil {
		return err
	}

	sectionName := profileSectionPrefix + profileName
	section := cfg.Section(sectionName)
	if section == nil {
		section, err = cfg.NewSection(sectionName)
		if err != nil {
			return err
		}
	}

	section.Key(keyName).SetValue(value)
	return cfg.SaveTo(path)
}
