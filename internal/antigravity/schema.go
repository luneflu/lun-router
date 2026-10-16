package antigravity

import (
	"fmt"
	"strings"
)

var unsupportedSchemaConstraints = map[string]bool{
	"minLength": true, "maxLength": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
	"minItems": true, "maxItems": true, "format": true, "multipleOf": true,
	"uniqueItems": true, "contains": true,
	"unevaluatedProperties": true, "unevaluatedItems": true, "contentSchema": true,
	"prefixItems": true, "additionalItems": true,
	"default": true, "examples": true,
	"$schema": true, "$defs": true, "definitions": true, "const": true, "$ref": true, "$comment": true,
	"deprecated": true, "readOnly": true, "writeOnly": true,
	"additionalProperties": true, "propertyNames": true, "patternProperties": true, "enumDescriptions": true,
	"anyOf": true, "oneOf": true, "allOf": true, "not": true,
	"dependencies": true, "dependentSchemas": true, "dependentRequired": true,
	"title": true, "optional": true, "if": true, "then": true, "else": true,
	"contentMediaType": true, "contentEncoding": true,
	"cornerRadius": true, "fillColor": true, "fontFamily": true, "fontSize": true, "fontWeight": true,
	"gap": true, "padding": true, "strokeColor": true, "strokeThickness": true, "textColor": true,
}

// CleanJSONSchemaForAntigravity sanitizes JSON schemas for Antigravity/Gemini backend (mirrors 9router cleanJSONSchemaForAntigravity)
func CleanJSONSchemaForAntigravity(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why you are calling this tool",
				},
			},
			"required": []string{"reason"},
		}
	}

	cleaned := deepCloneMap(schema)

	// Phase 1: Convert const to enum & enum values to string
	convertConstToEnum(cleaned)
	convertEnumValuesToStrings(cleaned)

	// Phase 2: Flatten complex structures (allOf, prefixItems, anyOf/oneOf, type arrays)
	mergeAllOf(cleaned)
	convertPrefixItems(cleaned)
	flattenAnyOfOneOf(cleaned)
	flattenTypeArrays(cleaned)

	// Phase 2.5: Infer missing type=object when properties exist, array items
	ensureObjectType(cleaned)
	ensureArrayItems(cleaned)

	// Phase 3: Remove unsupported keywords
	removeUnsupportedKeywords(cleaned)

	// Phase 4: Cleanup required fields
	cleanupRequired(cleaned)

	// Phase 5: Add placeholder for empty object schemas
	addPlaceholders(cleaned)

	return cleaned
}

func deepCloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	res := make(map[string]any, len(m))
	for k, v := range m {
		res[k] = deepCloneValue(v)
	}
	return res
}

func deepCloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCloneMap(val)
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = deepCloneValue(item)
		}
		return res
	default:
		return val
	}
}

func convertConstToEnum(obj map[string]any) {
	if obj == nil {
		return
	}
	if val, ok := obj["const"]; ok && obj["enum"] == nil {
		obj["enum"] = []any{val}
		delete(obj, "const")
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			convertConstToEnum(child)
		}
	}
}

func convertEnumValuesToStrings(obj map[string]any) {
	if obj == nil {
		return
	}
	if enums, ok := obj["enum"].([]any); ok {
		strEnums := make([]any, len(enums))
		for i, e := range enums {
			strEnums[i] = fmt.Sprint(e)
		}
		obj["enum"] = strEnums
		if _, ok := obj["type"]; !ok {
			obj["type"] = "string"
		}
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			convertEnumValuesToStrings(child)
		}
	}
}

func mergeAllOf(obj map[string]any) {
	if obj == nil {
		return
	}
	if allOf, ok := obj["allOf"].([]any); ok && len(allOf) > 0 {
		mergedProps := make(map[string]any)
		var mergedReq []string

		for _, item := range allOf {
			if m, ok := item.(map[string]any); ok {
				if props, ok := m["properties"].(map[string]any); ok {
					for pk, pv := range props {
						mergedProps[pk] = pv
					}
				}
				if req, ok := m["required"].([]any); ok {
					for _, r := range req {
						if rs, ok := r.(string); ok {
							mergedReq = append(mergedReq, rs)
						}
					}
				}
			}
		}

		delete(obj, "allOf")
		if len(mergedProps) > 0 {
			origProps, _ := obj["properties"].(map[string]any)
			if origProps == nil {
				origProps = make(map[string]any)
			}
			for pk, pv := range mergedProps {
				origProps[pk] = pv
			}
			obj["properties"] = origProps
		}
		if len(mergedReq) > 0 {
			origReq, _ := obj["required"].([]any)
			for _, r := range mergedReq {
				origReq = append(origReq, r)
			}
			obj["required"] = origReq
		}
	}

	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			mergeAllOf(child)
		}
	}
}

func convertPrefixItems(obj map[string]any) {
	if obj == nil {
		return
	}
	if prefixItems, ok := obj["prefixItems"].([]any); ok && len(prefixItems) > 0 {
		if obj["items"] == nil {
			var nonNull []map[string]any
			for _, pi := range prefixItems {
				if pim, ok := pi.(map[string]any); ok {
					if t, _ := pim["type"].(string); t != "null" {
						nonNull = append(nonNull, pim)
					}
				}
			}
			if len(nonNull) == 1 {
				obj["items"] = nonNull[0]
			} else if len(nonNull) > 1 {
				obj["items"] = map[string]any{"anyOf": nonNull}
			}
		}
		delete(obj, "prefixItems")
	}

	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			convertPrefixItems(child)
		}
	}
}

func flattenAnyOfOneOf(obj map[string]any) {
	if obj == nil {
		return
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if items, ok := obj[key].([]any); ok && len(items) > 0 {
			var candidates []map[string]any
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					if t, _ := m["type"].(string); t != "null" {
						candidates = append(candidates, m)
					}
				}
			}
			if len(candidates) > 0 {
				best := selectBestSchema(candidates)
				delete(obj, key)
				for k, v := range best {
					obj[k] = v
				}
			}
		}
	}

	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			flattenAnyOfOneOf(child)
		}
	}
}

func selectBestSchema(items []map[string]any) map[string]any {
	bestIdx := 0
	bestScore := -1

	for i, item := range items {
		score := 0
		t, _ := item["type"].(string)
		if t == "object" || item["properties"] != nil {
			score = 3
		} else if t == "array" || item["items"] != nil {
			score = 2
		} else if t != "" && t != "null" {
			score = 1
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return items[bestIdx]
}

func flattenTypeArrays(obj map[string]any) {
	if obj == nil {
		return
	}
	if typeList, ok := obj["type"].([]any); ok {
		var chosen string
		for _, t := range typeList {
			if ts, ok := t.(string); ok && ts != "null" {
				chosen = ts
				break
			}
		}
		if chosen == "" {
			chosen = "string"
		}
		obj["type"] = chosen
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			flattenTypeArrays(child)
		}
	}
}

func ensureObjectType(obj map[string]any) {
	if obj == nil {
		return
	}
	if obj["properties"] != nil && obj["type"] == nil {
		obj["type"] = "object"
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			ensureObjectType(child)
		}
	}
}

func ensureArrayItems(obj map[string]any) {
	if obj == nil {
		return
	}
	if t, _ := obj["type"].(string); t == "array" && obj["items"] == nil {
		obj["items"] = map[string]any{"type": "string"}
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			ensureArrayItems(child)
		}
	}
}

func removeUnsupportedKeywords(obj map[string]any) {
	if obj == nil {
		return
	}
	for k := range obj {
		if unsupportedSchemaConstraints[k] || strings.HasPrefix(k, "x-") {
			delete(obj, k)
		}
	}
	for _, v := range obj {
		switch child := v.(type) {
		case map[string]any:
			removeUnsupportedKeywords(child)
		case []any:
			for _, it := range child {
				if itm, ok := it.(map[string]any); ok {
					removeUnsupportedKeywords(itm)
				}
			}
		}
	}
}

func cleanupRequired(obj map[string]any) {
	if obj == nil {
		return
	}
	if obj["properties"] != nil {
		props, _ := obj["properties"].(map[string]any)
		var valid []any
		switch req := obj["required"].(type) {
		case []any:
			for _, r := range req {
				if rs, ok := r.(string); ok && props[rs] != nil {
					valid = append(valid, rs)
				}
			}
		case []string:
			for _, rs := range req {
				if props[rs] != nil {
					valid = append(valid, rs)
				}
			}
		}
		if obj["required"] != nil {
			if len(valid) == 0 {
				delete(obj, "required")
			} else {
				obj["required"] = valid
			}
		}
	}
	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			cleanupRequired(child)
		}
	}
}

func addPlaceholders(obj map[string]any) {
	if obj == nil {
		return
	}
	if len(obj) == 0 {
		obj["type"] = "object"
		obj["properties"] = map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Brief explanation of why you are calling this tool",
			},
		}
		obj["required"] = []any{"reason"}
		return
	}

	if t, _ := obj["type"].(string); t == "object" {
		props, _ := obj["properties"].(map[string]any)
		if len(props) == 0 {
			obj["properties"] = map[string]any{
				"reason": map[string]any{
					"type":        "string",
					"description": "Brief explanation of why you are calling this tool",
				},
			}
			obj["required"] = []any{"reason"}
		}
	}

	for _, v := range obj {
		if child, ok := v.(map[string]any); ok {
			addPlaceholders(child)
		}
	}
}
