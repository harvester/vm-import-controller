package util

import (
	"path/filepath"
	"strings"
)

func BaseName(path string) string {
	filename := filepath.Base(path)
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}
