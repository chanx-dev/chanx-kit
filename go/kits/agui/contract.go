package agui

import (
	"bytes"
	_ "embed"
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

// schema.json is the Python kit's, exported by scripts/contracts.py.
//
//go:embed schema.json
var contractJSON []byte

// contractSchema is the payload schema of a message, or a named $def, with every $def attached.
func contractSchema(name string, def bool) *jsonschema.Schema {
	var top map[string]json.RawMessage
	var raw struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	_ = json.Unmarshal(contractJSON, &top)
	_ = json.Unmarshal(contractJSON, &raw)
	defs := map[string]*jsonschema.Schema{}
	for n, r := range raw.Defs {
		defs[n] = parse(r)
	}
	s := parse(top[name])
	if def {
		cp := *defs[name]
		s = &cp
	}
	s.Defs = defs
	return s
}

func parse(raw json.RawMessage) *jsonschema.Schema {
	s := &jsonschema.Schema{}
	_ = json.Unmarshal(raw, s)
	order(s, raw)
	return s
}

// order sets PropertyOrder from the JSON, since the schema type drops key order.
func order(s *jsonschema.Schema, raw json.RawMessage) {
	if s == nil || len(raw) == 0 {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	if props, ok := fields["properties"]; ok {
		s.PropertyOrder = keys(props)
		var each map[string]json.RawMessage
		_ = json.Unmarshal(props, &each)
		for name, sub := range s.Properties {
			order(sub, each[name])
		}
	}
	order(s.Items, fields["items"])
	order(s.AdditionalProperties, fields["additionalProperties"])
	for key, list := range map[string][]*jsonschema.Schema{"anyOf": s.AnyOf, "oneOf": s.OneOf, "allOf": s.AllOf} {
		var each []json.RawMessage
		_ = json.Unmarshal(fields[key], &each)
		for i, sub := range list {
			if i < len(each) {
				order(sub, each[i])
			}
		}
	}
}

func keys(object json.RawMessage) []string {
	dec := json.NewDecoder(bytes.NewReader(object))
	_, _ = dec.Token()
	var out []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		out = append(out, tok.(string))
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return out
		}
	}
	return out
}
