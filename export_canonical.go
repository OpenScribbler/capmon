package capmon

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// canonicalJSON serializes doc under capmon's pinned canonicalization profile
// and is the sole JSON writer for the export tree. encoding/json sorts map keys
// byte-wise (Unicode code-point order); SetEscapeHTML(false) keeps &<> raw and
// non-ASCII as UTF-8; SetIndent gives two-space indentation and Encode appends a
// single trailing LF. Floats are rejected anywhere in the tree — exported
// documents carry integers only.
func canonicalJSON(doc map[string]any) ([]byte, error) {
	if err := rejectFloats(doc); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rejectFloats walks v and errors on any float32/float64 value.
func rejectFloats(v any) error {
	switch t := v.(type) {
	case float64, float32:
		return fmt.Errorf("canonicalJSON: float value not permitted (integers only): %v", v)
	case map[string]any:
		for _, e := range t {
			if err := rejectFloats(e); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := rejectFloats(e); err != nil {
				return err
			}
		}
	}
	return nil
}
