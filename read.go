package fread

import (
	"encoding/json"
	"fmt"
	"os"
)

type Read struct{}

func (r Read) Read(path string, value any) error {
	data, err := os.ReadFile(path)

	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return json.Unmarshal(data, value)
}
