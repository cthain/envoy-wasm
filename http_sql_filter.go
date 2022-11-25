package main

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
)

// SQLFilter is a rudimentary filter that attempts to detect SQL injection patterns
// in HTTP request headers and bodies.
// It operates on a configurable slice of keywords and compares each to the values
// of the request headers and body using a simple, case-insensitive string match.
// If one of the configured keywords is found, the request is rejected.
type SQLFilter struct {
	keywords []string
}

func NewSQLFilter(keywords []string) *SQLFilter {
	proxywasm.LogInfo("creating new SQLFilter")
	return &SQLFilter{keywords: keywords}
}

func (f *SQLFilter) FilterHeaders(headers map[string][]string) (int, error) {
	if values, exists := headers[":path"]; exists {
		for _, v := range values {
			if err := f.detectSQLInjection(v); err != nil {
				return StatusBadRequest, fmt.Errorf("possible SQL injection detected: %w", err)
			}
		}
	} else {
		return StatusBadRequest, fmt.Errorf("path header missing from request")
	}
	return StatusOK, nil
}

func (f *SQLFilter) FilterBody(body []byte) (int, error) {
	if err := f.detectSQLInjection(string(body)); err != nil {
		return StatusBadRequest, fmt.Errorf("possible SQL injection detected: %w", err)
	}
	return StatusOK, nil
}

func (f *SQLFilter) detectSQLInjection(s string) error {
	for _, kw := range f.keywords {
		if strings.Contains(strings.ToLower(s), kw) {
			return fmt.Errorf("SQL keyword '%s' found", kw)
		}
	}
	return nil
}
