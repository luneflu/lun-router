package zai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// streamFrame is one `data:` payload from POST /api/v2/chat/completions.
//
// The service emits three shapes:
//
//	thinking -> delta_content (append)
//	answer   -> edit_index + edit_content (replace from that index)
//	other    -> usage
//	done     -> done:true
//
// The answer arriving as an edit rather than an append is the trap in this
// protocol: a parser that only reads delta_content returns an empty reply.
type streamFrame struct {
	Type string `json:"type"`
	Data struct {
		Phase        string          `json:"phase"`
		DeltaContent string          `json:"delta_content"`
		EditContent  string          `json:"edit_content"`
		EditIndex    *int            `json:"edit_index"`
		Done         bool            `json:"done"`
		Usage        json.RawMessage `json:"usage"`
		Error        json.RawMessage `json:"error"`
		Data         struct {
			Error json.RawMessage `json:"error"`
			Done  bool            `json:"done"`
		} `json:"data"`
	} `json:"data"`
	Error json.RawMessage `json:"error"`
}

// frameError returns the upstream error payload, whichever nesting carries it.
func (f streamFrame) frameError() json.RawMessage {
	for _, raw := range []json.RawMessage{f.Data.Error, f.Data.Data.Error, f.Error} {
		if len(raw) > 0 && string(raw) != "null" {
			return raw
		}
	}
	return nil
}

// decoder folds a frame stream into the message buffer. The buffer holds the
// whole reply including the reasoning block, because edit_index is an offset
// into that combined text.
type decoder struct {
	buf      string
	usage    json.RawMessage
	errRaw   json.RawMessage
	done     bool
	thinking bool
	// answerStart is the buffer offset at which the visible answer began. It is
	// needed because the service streams reasoning and answer into one text run
	// when it does not wrap the reasoning in a <details> block.
	answerStart int
	answerSet   bool
}

func (d *decoder) apply(f streamFrame) {
	if raw := f.frameError(); raw != nil {
		d.errRaw = raw
	}

	// Record where the answer begins before writing this frame, so an
	// answer-phase delta does not absorb the tail of the reasoning.
	if f.Data.Phase == "answer" && !d.answerSet {
		switch {
		case f.Data.EditIndex != nil:
			d.answerStart = *f.Data.EditIndex
		default:
			d.answerStart = len(d.buf)
		}
		d.answerSet = true
	}

	switch {
	case f.Data.EditIndex != nil && f.Data.EditContent != "":
		idx := *f.Data.EditIndex
		if idx < 0 {
			idx = 0
		}
		if idx > len(d.buf) {
			// The service should never reference past the end of what it has
			// already sent; if it does, prefer appending to dropping text.
			d.buf += f.Data.EditContent
			break
		}
		d.buf = d.buf[:idx] + f.Data.EditContent
	case f.Data.DeltaContent != "":
		d.buf += f.Data.DeltaContent
	}
	if f.Data.Phase == "other" && len(f.Data.Usage) > 0 {
		d.usage = f.Data.Usage
	}
	if f.Data.Done || f.Data.Data.Done {
		d.done = true
	}
}

// Answer returns the visible reply.
//
// Two shapes reach us. With a <details> reasoning block inlined, the answer is
// what follows the closing tag. Without one, the service interleaves reasoning
// and answer in a single run separated only by the phase field, so the answer is
// the tail from where the answer phase began. Using one rule alone leaks the
// model's private reasoning into the reply.
func (d *decoder) Answer() (answer, reasoning string) {
	if strings.Contains(d.buf, thinkingOpen) {
		return SplitReasoning(d.buf)
	}
	if d.answerSet && d.answerStart > 0 && d.answerStart <= len(d.buf) {
		return strings.TrimSpace(d.buf[d.answerStart:]), strings.TrimSpace(d.buf[:d.answerStart])
	}
	return strings.TrimSpace(d.buf), ""
}

// thinkingOpen / thinkingClose bracket the reasoning block the service inlines
// ahead of the answer.
const (
	thinkingOpen  = "<details type=\"reasoning\""
	thinkingClose = "</details>"
)

// SplitReasoning separates the inlined reasoning block from the answer and
// returns both. The answer is everything after the closing tag.
func SplitReasoning(full string) (answer, reasoning string) {
	start := strings.Index(full, thinkingOpen)
	if start == -1 {
		return strings.TrimSpace(full), ""
	}
	end := strings.Index(full[start:], thinkingClose)
	if end == -1 {
		// Reasoning opened but never closed: treat the tail as reasoning so a
		// truncated block cannot leak into the visible answer.
		return strings.TrimSpace(full[:start]), strings.TrimSpace(full[start:])
	}
	closeAt := start + end + len(thinkingClose)
	return strings.TrimSpace(full[closeAt:]), strings.TrimSpace(full[start:closeAt])
}

// readStream consumes an SSE body, invoking onAnswer whenever the visible
// answer grows. onAnswer receives the full answer so far, so callers can emit
// deltas for streaming clients or ignore it entirely.
func readStream(r io.Reader, onAnswer func(string)) (*decoder, error) {
	d := &decoder{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 4<<20)

	last := ""
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[len("data:"):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var f streamFrame
		if err := json.Unmarshal([]byte(payload), &f); err != nil {
			continue
		}
		if debugEnabled() {
			fmt.Fprintf(os.Stderr, "[zai-frame] phase=%s delta=%.60q edit_idx=%v edit=%.80q\n",
				f.Data.Phase, f.Data.DeltaContent, f.Data.EditIndex, f.Data.EditContent)
		}
		d.apply(f)

		if onAnswer != nil {
			answer, _ := d.Answer()
			if answer != last {
				last = answer
				onAnswer(answer)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return d, err
	}
	return d, nil
}
