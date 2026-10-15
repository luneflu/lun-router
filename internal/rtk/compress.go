package rtk

import (
	"fmt"
	"log"
	"strings"
)

type Hit struct {
	Shape  string `json:"shape"`
	Filter string `json:"filter"`
	Saved  int    `json:"saved"`
}

type Stats struct {
	BytesBefore int   `json:"bytes_before"`
	BytesAfter  int   `json:"bytes_after"`
	Hits        []Hit `json:"hits"`
}

func (s *Stats) LogLine() string {
	if s == nil || len(s.Hits) == 0 {
		return ""
	}
	saved := s.BytesBefore - s.BytesAfter
	pct := 0.0
	if s.BytesBefore > 0 {
		pct = (float64(saved) / float64(s.BytesBefore)) * 100.0
	}
	filterSet := make(map[string]bool)
	for _, h := range s.Hits {
		filterSet[h.Filter] = true
	}
	var filters []string
	for f := range filterSet {
		filters = append(filters, f)
	}
	return fmt.Sprintf("[RTK] saved %dB / %dB (%.1f%%) via [%s] hits=%d", saved, s.BytesBefore, pct, strings.Join(filters, ","), len(s.Hits))
}

func SafeApply(fn FilterFunc, text string) (out string) {
	if fn == nil {
		return text
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[RTK] warning: filter panicked, passing through raw output: %v", r)
			out = text
		}
	}()
	return fn(text)
}

func CompressText(text string, stats *Stats, shape string) string {
	bytesIn := len(text)
	if stats != nil {
		stats.BytesBefore += bytesIn
	}

	if bytesIn < MinCompressSize || bytesIn > RawCap {
		if stats != nil {
			stats.BytesAfter += bytesIn
		}
		return text
	}

	fn, filterName := AutoDetectFilter(text)
	if fn == nil {
		if stats != nil {
			stats.BytesAfter += bytesIn
		}
		return text
	}

	out := SafeApply(fn, text)

	// Safety: never return empty, never grow the input
	if len(out) == 0 || len(out) >= bytesIn {
		if stats != nil {
			stats.BytesAfter += bytesIn
		}
		return text
	}

	if stats != nil {
		stats.BytesAfter += len(out)
		stats.Hits = append(stats.Hits, Hit{
			Shape:  shape,
			Filter: filterName,
			Saved:  bytesIn - len(out),
		})
	}
	return out
}

// CompressMessages compresses tool_result / tool content in-place in bodyMap.
// Returns Stats or nil if disabled or nothing compressed.
func CompressMessages(body map[string]any, enabled bool) *Stats {
	if !enabled || body == nil {
		return nil
	}

	rawItems, ok := body["messages"]
	if !ok {
		rawItems, ok = body["input"]
	}
	if !ok {
		return nil
	}

	items, ok := rawItems.([]any)
	if !ok {
		return nil
	}

	stats := &Stats{}

	for _, rawMsg := range items {
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			continue
		}

		// Shape 4: OpenAI Responses — { type:"function_call_output", output: ... }
		if msgType, _ := msg["type"].(string); msgType == "function_call_output" {
			if outStr, ok := msg["output"].(string); ok {
				msg["output"] = CompressText(outStr, stats, "openai-responses-string")
			} else if outArr, ok := msg["output"].([]any); ok {
				for _, part := range outArr {
					if pMap, ok := part.(map[string]any); ok {
						if pType, _ := pMap["type"].(string); pType == "input_text" {
							if text, ok := pMap["text"].(string); ok {
								pMap["text"] = CompressText(text, stats, "openai-responses-array")
							}
						}
					}
				}
			}
			continue
		}

		role, _ := msg["role"].(string)

		// Shape 1: OpenAI tool message — { role:"tool", content: "string" }
		if role == "tool" {
			if contentStr, ok := msg["content"].(string); ok {
				msg["content"] = CompressText(contentStr, stats, "openai-tool")
				continue
			}
		}

		// Content array
		contentArr, ok := msg["content"].([]any)
		if !ok {
			continue
		}

		// Shape 1b: OpenAI tool message — { role:"tool", content: [{type:"text", text:"..."}] }
		if role == "tool" {
			for _, part := range contentArr {
				if pMap, ok := part.(map[string]any); ok {
					if pType, _ := pMap["type"].(string); pType == "text" {
						if text, ok := pMap["text"].(string); ok {
							pMap["text"] = CompressText(text, stats, "openai-tool-array")
						}
					}
				}
			}
			continue
		}

		// Shape 2/3: blocks array with tool_result entries (e.g. Claude format / Anthropic tool_result)
		for _, block := range contentArr {
			bMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			bType, _ := bMap["type"].(string)
			if bType != "tool_result" {
				continue
			}
			// Skip errors to preserve full traces
			if isErr, _ := bMap["is_error"].(bool); isErr {
				continue
			}
			if status, _ := bMap["status"].(string); status == "error" {
				continue
			}

			if bStr, ok := bMap["content"].(string); ok {
				bMap["content"] = CompressText(bStr, stats, "claude-string")
			} else if bArr, ok := bMap["content"].([]any); ok {
				for _, part := range bArr {
					if pMap, ok := part.(map[string]any); ok {
						if pType, _ := pMap["type"].(string); pType == "text" {
							if text, ok := pMap["text"].(string); ok {
								pMap["text"] = CompressText(text, stats, "claude-array")
							}
						}
					}
				}
			}
		}
	}

	if len(stats.Hits) == 0 {
		return nil
	}
	return stats
}
