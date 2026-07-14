package capmon

import (
	"fmt"

	"github.com/OpenScribbler/capmon/capyaml"
	"github.com/OpenScribbler/capmon/internal/output"
)

// buildProviderDoc compiles one provider's capability baseline joined with the
// canonical-key registry into a document map ready for canonicalJSON. It fails
// closed with EXPORT_001 on any non-canonical node in canonical-key position.
func buildProviderDoc(caps *capyaml.ProviderCapabilities, reg keyRegistry) (map[string]any, error) {
	doc := map[string]any{
		"schema_version": "1",
		// status is the published document's lifecycle (live → frozen/removed
		// per the consumer contract), not the provider product's health — that
		// is joined from the source manifest as provider_status.
		"status": "live",
		"slug":   caps.Slug,
	}
	displayName := caps.DisplayName
	if displayName == "" {
		displayName = caps.Slug
	}
	doc["display_name"] = displayName
	if caps.LastVerified != "" {
		doc["last_verified"] = caps.LastVerified
	}

	if len(caps.ContentTypes) > 0 {
		cts := make(map[string]any, len(caps.ContentTypes))
		for name, ct := range caps.ContentTypes {
			node, err := buildContentTypeNode(caps.Slug, name, ct, reg)
			if err != nil {
				return nil, err
			}
			cts[name] = node
		}
		doc["content_types"] = cts
	}

	if len(caps.References) > 0 {
		refs := make(map[string]any, len(caps.References))
		for id, r := range caps.References {
			entry := map[string]any{"url": r.URL}
			if r.VerifiedAt != "" {
				entry["verified_at"] = r.VerifiedAt
			}
			refs[id] = entry
		}
		doc["references"] = refs
	}

	if len(caps.ProviderExclusive) > 0 {
		pe := make(map[string]any, len(caps.ProviderExclusive))
		for name, ce := range caps.ProviderExclusive {
			pe[name] = buildCapabilityNode(ce)
		}
		doc["provider_exclusive"] = pe
	}

	return doc, nil
}

// buildContentTypeNode builds one content-type node. Direct children of the
// capabilities map are registry-backed: each carries key_path + key metadata,
// and an unregistered one fails closed.
func buildContentTypeNode(slug, ctName string, ct capyaml.ContentTypeEntry, reg keyRegistry) (map[string]any, error) {
	node := map[string]any{"supported": ct.Supported}
	if ct.Confidence != "" {
		node["confidence"] = ct.Confidence
	}

	if len(ct.Events) > 0 {
		events := make(map[string]any, len(ct.Events))
		for name, e := range ct.Events {
			ev := map[string]any{"native_name": e.NativeName}
			if e.Blocking != "" {
				ev["blocking"] = e.Blocking
			}
			if len(e.Refs) > 0 {
				ev["refs"] = e.Refs
			}
			events[name] = ev
		}
		node["events"] = events
	}

	if len(ct.Tools) > 0 {
		tools := make(map[string]any, len(ct.Tools))
		for name, tl := range ct.Tools {
			t := map[string]any{"native": tl.Native}
			if len(tl.Refs) > 0 {
				t["refs"] = tl.Refs
			}
			tools[name] = t
		}
		node["tools"] = tools
	}

	if len(ct.Capabilities) > 0 {
		ctReg := reg[ctName]
		caps := make(map[string]any, len(ct.Capabilities))
		for name, ce := range ct.Capabilities {
			meta, ok := ctReg[name]
			if !ok {
				return nil, output.NewStructuredError(
					"EXPORT_001",
					fmt.Sprintf("provider %q: non-canonical node %q in canonical-key position at content_types.%s.capabilities", slug, name, ctName),
					fmt.Sprintf("Relocate %q to provider_exclusive or register it as a canonical key for content type %q.", name, ctName),
				)
			}
			child := buildCapabilityNode(ce)
			child["key_path"] = ctName + "." + name
			child["key"] = map[string]any{
				"description": meta.Description,
				"type":        meta.Type,
				"spec_ref":    meta.SpecRef,
			}
			caps[name] = child
		}
		node["capabilities"] = caps
	}

	return node, nil
}

// buildCapabilityNode mirrors a recursive CapabilityEntry into a document node.
// These nodes are vocabulary members (or provider_exclusive nodes): supported is
// always emitted, and they definitionally never carry key/key_path.
func buildCapabilityNode(ce capyaml.CapabilityEntry) map[string]any {
	node := map[string]any{"supported": ce.Supported}
	if ce.Mechanism != "" {
		node["mechanism"] = ce.Mechanism
	}
	if ce.Confidence != "" {
		node["confidence"] = ce.Confidence
	}
	if len(ce.Refs) > 0 {
		node["refs"] = ce.Refs
	}
	if len(ce.Capabilities) > 0 {
		children := make(map[string]any, len(ce.Capabilities))
		for name, child := range ce.Capabilities {
			children[name] = buildCapabilityNode(child)
		}
		node["capabilities"] = children
	}
	return node
}
