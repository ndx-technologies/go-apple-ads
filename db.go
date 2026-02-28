package goappleads

import (
	"encoding/json/v2"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func Load(path string) (*Config, *KeywordCSVDB, error) {
	var config Config

	f, err := os.Open(filepath.Join(path, "config.json"))
	if err != nil {
		slog.Error("cannot open file", "file", path, "error", err)
		return nil, nil, err
	}
	defer f.Close()

	if err := json.UnmarshalRead(f, &config); err != nil {
		return nil, nil, err
	}

	config.Init()

	var keywordsDB KeywordCSVDB

	entries, err := os.ReadDir(path + "/keywords")
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}
		f, err := os.Open(filepath.Join(path, "keywords", entry.Name()))
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		if err := keywordsDB.LoadFromCSV(f); err != nil {
			return nil, nil, err
		}
	}

	return &config, &keywordsDB, nil
}
