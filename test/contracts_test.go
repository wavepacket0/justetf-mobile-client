package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		t.Fatalf("live verification requested but %s is missing", name)
	}
	return value
}

func requireLiveSuccess(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var apiError *justetf.APIError
	if errors.As(err, &apiError) {
		t.Fatalf("live request failed: HTTP %d", apiError.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("live request timed out")
	}
	t.Fatal("live request failed; transport or client error (details withheld)")
}

func checkResponseFields(raw json.RawMessage, fields map[string]string) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil || !json.Valid(raw) {
		return nil, errors.New("expected a JSON object")
	}
	for field, accepted := range fields {
		value, exists := object[field]
		if !exists {
			return nil, fmt.Errorf("required field %s is missing", field)
		}
		kind := "null"
		switch value.(type) {
		case string:
			kind = "string"
		case json.Number:
			kind = "number"
		case bool:
			kind = "boolean"
		case []any:
			kind = "array"
		case map[string]any:
			kind = "object"
		}
		if !strings.Contains("|"+accepted+"|", "|"+kind+"|") {
			return nil, fmt.Errorf("field %s: expected %s, received %s", field, accepted, kind)
		}
	}
	return object, nil
}

func TestLiveResponseValidation(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		raw   string
		valid bool
	}{
		{"expected", `{"name":"ETF","price":1.234567890123456789,"extra":true}`, true},
		{"nullable", `{"name":"ETF","price":null}`, true},
		{"missing", `{"name":"ETF"}`, false},
		{"wrong_type", `{"name":42,"price":1}`, false},
		{"wrong_nullable_type", `{"name":"ETF","price":"1"}`, false},
		{"array", `[]`, false},
		{"null", `null`, false},
		{"malformed", `{`, false},
		{"trailing_json", `{"name":"ETF","price":1} {}`, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := checkResponseFields(json.RawMessage(scenario.raw), map[string]string{"name": "string", "price": "number|null"})
			if (err == nil) != scenario.valid {
				t.Fatal("unexpected contract validation result")
			}
		})
	}
}
