package main

import (
	"encoding/json"
	"flag"
	"os"
	"strconv"
)

func applyEnvOverrides(config Config) Config {
	if val := os.Getenv("BROWSER_READER_MODE"); val != "" {
		config.EnableReaderMode = val == "true"
	}
	if val := os.Getenv("BROWSER_MAX_TABS"); val != "" {
		if max, err := strconv.Atoi(val); err == nil {
			config.MaxTabs = max
		}
	}
	if val := os.Getenv("BROWSER_BOOKMARKS"); val != "" {
		config.EnableBookmarks = val == "true"
	}
	if val := os.Getenv("BROWSER_HISTORY"); val != "" {
		config.EnableHistory = val == "true"
	}
	if val := os.Getenv("BROWSER_SEARCH"); val != "" {
		config.EnableSearch = val == "true"
	}
	if val := os.Getenv("BROWSER_STATUS_PANEL"); val != "" {
		config.EnableStatusPanel = val == "true"
	}

	return config
}

func ParseFlags() Config {
	config := DefaultConfig()

	var configFile string
	flag.StringVar(&configFile, "config", "", "Path to config file")
	flag.BoolVar(&config.EnableReaderMode, "reader", config.EnableReaderMode, "Enable reader mode")
	flag.BoolVar(&config.EnableBookmarks, "bookmarks", config.EnableBookmarks, "Enable bookmarks")
	flag.BoolVar(&config.EnableTabs, "tabs", config.EnableTabs, "Enable tabs")
	flag.IntVar(&config.MaxTabs, "max-tabs", config.MaxTabs, "Maximum tabs")
	flag.BoolVar(&config.EnableHistory, "history", config.EnableHistory, "Enable history")
	flag.BoolVar(&config.EnableSearch, "search", config.EnableSearch, "Enable search")
	flag.BoolVar(
		&config.EnableStatusPanel,
		"status",
		config.EnableStatusPanel,
		"Enable status panel",
	)

	flag.Parse()

	// If config file specified, load it (overrides flag defaults)
	if configFile != "" {
		if fileConfig, err := loadConfigFromFile(configFile); err == nil {
			config = fileConfig
		}
	}

	return config
}

func loadConfigFromFile(filename string) (Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return DefaultConfig(), err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return DefaultConfig(), err
	}
	return config, nil
}
