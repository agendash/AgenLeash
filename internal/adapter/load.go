package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func LoadFile(path string) (AdapterSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AdapterSpec{}, err
	}

	var spec AdapterSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return AdapterSpec{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := spec.Validate(); err != nil {
		return AdapterSpec{}, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

func LoadDirectory(root string) ([]AdapterSpec, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var specs []AdapterSpec
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		spec, err := LoadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Metadata.Name == specs[j].Metadata.Name {
			return specs[i].Metadata.AdapterRevision < specs[j].Metadata.AdapterRevision
		}
		return specs[i].Metadata.Name < specs[j].Metadata.Name
	})

	return specs, nil
}
