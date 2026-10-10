package membershippaymentmethod

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
)

// editorDoc is the Editor.js document shape the renderer understands.
type editorDoc struct {
	Blocks []editorBlock `json:"blocks"`
}

type editorBlock struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// RenderEditorHTML converts Editor.js JSON (as stored by the editor field) to
// HTML for display, mirroring the reference RendersEditorContent trait. A
// plain-text value (not starting with '{') is escaped and shown as-is. Every
// interpolated value is HTML-escaped, matching the reference's e() calls.
func RenderEditorHTML(raw *string) string {
	if raw == nil || *raw == "" {
		return ""
	}
	content := *raw
	if content[0] != '{' {
		return html.EscapeString(content)
	}
	var doc editorDoc
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return html.EscapeString(content)
	}
	var b strings.Builder
	for _, block := range doc.Blocks {
		switch block.Type {
		case "paragraph":
			b.WriteString("<p>" + html.EscapeString(editorText(block.Data, "text")) + "</p>")
		case "header":
			b.WriteString(renderEditorHeader(block.Data))
		case "list":
			b.WriteString(renderEditorList(block.Data))
		case "table":
			b.WriteString(renderEditorTable(block.Data))
		case "image":
			b.WriteString(renderEditorImage(block.Data))
		}
	}
	return b.String()
}

// renderEditorHeader renders a header block (level clamped to 1..6).
func renderEditorHeader(data json.RawMessage) string {
	level := intEditorNum(data, "level")
	if level < 1 {
		level = 2
	}
	if level > 6 {
		level = 6
	}
	text := html.EscapeString(editorText(data, "text"))
	return "<h" + strconv.Itoa(level) + ">" + text + "</h" + strconv.Itoa(level) + ">"
}

// renderEditorList renders an ordered/unordered list block.
func renderEditorList(data json.RawMessage) string {
	var d struct {
		Style string            `json:"style"`
		Items []json.RawMessage `json:"items"`
	}
	_ = json.Unmarshal(data, &d)
	tag := "ul"
	if d.Style == "ordered" {
		tag = "ol"
	}
	var b strings.Builder
	b.WriteString("<" + tag + ">")
	for _, rawItem := range d.Items {
		b.WriteString("<li>" + html.EscapeString(editorItemText(rawItem)) + "</li>")
	}
	b.WriteString("</" + tag + ">")
	return b.String()
}

// renderEditorTable renders a table block (first row is the header row).
func renderEditorTable(data json.RawMessage) string {
	var d struct {
		Content [][]string `json:"content"`
	}
	_ = json.Unmarshal(data, &d)
	if len(d.Content) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<table>")
	for i, cells := range d.Content {
		b.WriteString("<tr>")
		for _, cell := range cells {
			tag := "td"
			if i == 0 {
				tag = "th"
			}
			b.WriteString("<" + tag + ">" + html.EscapeString(cell) + "</" + tag + ">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// renderEditorImage renders an image block (src + alt caption).
func renderEditorImage(data json.RawMessage) string {
	var d struct {
		File struct {
			URL string `json:"url"`
		} `json:"file"`
		Caption string `json:"caption"`
	}
	_ = json.Unmarshal(data, &d)
	return `<img src="` + html.EscapeString(d.File.URL) + `" alt="` + html.EscapeString(d.Caption) + `">`
}

// editorItemText renders one list item: a bare string, or an object with a
// content field (reference (string)($item->content ?? $item)).
func editorItemText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(raw, &obj)
	return obj.Content
}

// editorText pulls a string field from a block's data object.
func editorText(data json.RawMessage, key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// intEditorNum pulls a numeric field from a block's data object.
func intEditorNum(data json.RawMessage, key string) int {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
