package jsonc

import (
	"bytes"
	"encoding/json"
	"io"
)

// Parse reads JSON with comments (JSONC) and returns standard JSON-decoded data.
// It strips // and /* */ comments before parsing.
func Parse(r io.Reader, v interface{}) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Strip // line comments
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if idx := bytes.Index(line, []byte("//")); idx != -1 {
			lines[i] = line[:idx]
		}
	}
	data = bytes.Join(lines, []byte("\n"))

	// Strip /* */ block comments
	var result []byte
	inComment := false
	for i := 0; i < len(data); i++ {
		if i < len(data)-1 && data[i] == '/' && data[i+1] == '*' {
			inComment = true
			i++
			continue
		}
		if i < len(data)-1 && data[i] == '*' && data[i+1] == '/' {
			inComment = false
			i++
			continue
		}
		if !inComment {
			result = append(result, data[i])
		}
	}

	return json.Unmarshal(result, v)
}