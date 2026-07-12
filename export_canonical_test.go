package capmon

import (
	"bytes"
	"strings"
	"testing"
)

func TestCanonicalJSONProfile(t *testing.T) {
	tests := []struct {
		name    string
		doc     map[string]any
		wantErr bool
		check   func(t *testing.T, out []byte)
	}{
		{
			name: "keys sorted ascending by code point including non-ASCII",
			// 'é' (U+00E9) sorts after 'z' (U+007A); both sort after 'a'.
			doc: map[string]any{
				"zeta":   1,
				"alpha":  2,
				"émigré": 3,
			},
			check: func(t *testing.T, out []byte) {
				iAlpha := bytes.Index(out, []byte(`"alpha"`))
				iZeta := bytes.Index(out, []byte(`"zeta"`))
				iEmigre := bytes.Index(out, []byte(`"émigré"`))
				if iAlpha < 0 || iZeta < 0 || iEmigre < 0 {
					t.Fatalf("missing key in output: %s", out)
				}
				if !(iAlpha < iZeta && iZeta < iEmigre) {
					t.Errorf("keys not sorted by code point: alpha@%d zeta@%d émigré@%d\n%s", iAlpha, iZeta, iEmigre, out)
				}
			},
		},
		{
			name: "two-space indent, single trailing LF, no CR",
			doc: map[string]any{
				"outer": map[string]any{"inner": 1},
			},
			check: func(t *testing.T, out []byte) {
				if bytes.Contains(out, []byte("\r")) {
					t.Error("output contains a carriage return")
				}
				if len(out) == 0 || out[len(out)-1] != '\n' {
					t.Fatalf("output does not end with LF: %q", out)
				}
				if len(out) >= 2 && out[len(out)-2] == '\n' {
					t.Errorf("output ends with more than one trailing LF: %q", out)
				}
				// Two-space indent for a first-level key.
				if !bytes.Contains(out, []byte("\n  \"outer\"")) {
					t.Errorf("expected two-space indent before first-level key:\n%s", out)
				}
				// Four-space indent for the second level.
				if !bytes.Contains(out, []byte("\n    \"inner\"")) {
					t.Errorf("expected four-space indent for nested key:\n%s", out)
				}
			},
		},
		{
			name: "ampersand, angle brackets emitted unescaped",
			doc:  map[string]any{"expr": "a & b < c > d"},
			check: func(t *testing.T, out []byte) {
				if !bytes.Contains(out, []byte("a & b < c > d")) {
					t.Errorf("HTML characters were escaped:\n%s", out)
				}
				for _, esc := range []string{"\\u0026", "\\u003c", "\\u003e"} {
					if bytes.Contains(out, []byte(esc)) {
						t.Errorf("found escape sequence %s in output:\n%s", esc, out)
					}
				}
			},
		},
		{
			name: "non-ASCII emitted as raw UTF-8",
			doc:  map[string]any{"name": "émigré café"},
			check: func(t *testing.T, out []byte) {
				if !bytes.Contains(out, []byte("émigré café")) {
					t.Errorf("non-ASCII value not emitted as raw UTF-8:\n%s", out)
				}
				if bytes.Contains(out, []byte("\\u00e9")) {
					t.Errorf("non-ASCII value was \\u-escaped:\n%s", out)
				}
			},
		},
		{
			name: "int emitted bare without decimal point",
			doc:  map[string]any{"count": 42},
			check: func(t *testing.T, out []byte) {
				if !bytes.Contains(out, []byte("42")) {
					t.Errorf("integer value missing:\n%s", out)
				}
				if bytes.Contains(out, []byte(".")) {
					t.Errorf("integer emitted with a decimal point:\n%s", out)
				}
			},
		},
		{
			name: "nested maps sorted at every level",
			doc: map[string]any{
				"b_top": map[string]any{
					"z_child": 1,
					"a_child": 2,
				},
				"a_top": 3,
			},
			check: func(t *testing.T, out []byte) {
				s := string(out)
				if strings.Index(s, `"a_top"`) > strings.Index(s, `"b_top"`) {
					t.Errorf("top-level keys not sorted:\n%s", out)
				}
				if strings.Index(s, `"a_child"`) > strings.Index(s, `"z_child"`) {
					t.Errorf("nested keys not sorted:\n%s", out)
				}
			},
		},
		{
			name:    "top-level float64 returns error",
			doc:     map[string]any{"ratio": 3.14},
			wantErr: true,
		},
		{
			name: "nested float64 returns error",
			doc: map[string]any{
				"outer": map[string]any{"inner": 1.5},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := canonicalJSON(tt.doc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got output:\n%s", out)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonicalJSON: %v", err)
			}
			tt.check(t, out)
		})
	}
}
