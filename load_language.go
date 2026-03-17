package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io"
)

// LoadLanguage deserializes a compressed grammar blob into a Language.
// Blobs are produced by grammargen's GenerateLanguage or the grammar
// build toolchain. This is the only function needed at runtime to load
// pre-compiled grammars — no grammargen import required.
func LoadLanguage(data []byte) (*Language, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	raw, err := io.ReadAll(gzr)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}

	var lang Language
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&lang); err != nil {
		return nil, fmt.Errorf("decode language: %w", err)
	}

	return &lang, nil
}
