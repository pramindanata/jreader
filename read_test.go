package fread_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pramindanata/fread"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_ValidJSON(t *testing.T) {
	path := writeTempJSON(t, "valid.json", `{"name":"Alice","age":30,"roles":["admin","user"]}`)

	fileReader := fread.Read{}

	var got testData
	require.NoError(t, fileReader.Read(path, &got))

	want := testData{Name: "Alice", Age: 30, Roles: []string{"admin", "user"}}
	assert.Equal(t, want, got)
}

func TestRead_IntoMap(t *testing.T) {
	path := writeTempJSON(t, "map.json", `{"key":"value","count":42}`)

	fileReader := fread.Read{}

	var got map[string]any
	require.NoError(t, fileReader.Read(path, &got))

	assert.Equal(t, "value", got["key"])
	assert.Equal(t, float64(42), got["count"])
}

func TestRead_InvalidJSON(t *testing.T) {
	path := writeTempJSON(t, "invalid.json", `{"name":`)

	fileReader := fread.Read{}

	var got testData
	err := fileReader.Read(path, &got)

	var syntaxErr *json.SyntaxError
	require.Error(t, err)
	assert.ErrorAs(t, err, &syntaxErr)
}

func TestRead_FileNotFound(t *testing.T) {
	path := filepath.Join("temp", "does-not-exist.json")
	if err := os.Remove(path); err != nil {
		require.ErrorIs(t, err, os.ErrNotExist)
	}

	fileReader := fread.Read{}

	var got testData
	require.ErrorIs(t, fileReader.Read(path, &got), os.ErrNotExist)
}

type testData struct {
	Name  string   `json:"name"`
	Age   int      `json:"age"`
	Roles []string `json:"roles"`
}

func writeTempJSON(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join("temp", name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	t.Cleanup(func() {
		require.NoError(t, os.Remove(path))
	})

	return path
}
