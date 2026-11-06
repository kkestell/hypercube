package main

import (
	"encoding/json"
	"strings"

	"github.com/kkestell/hypercube/internal/harness"
)

type fileChange struct {
	Path string          `json:"path"`
	Kind json.RawMessage `json:"kind"`
	Diff string          `json:"diff"`
}

type toolResult struct {
	Content           []contentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`     // an MCP image block's base64 data
	MIMEType string `json:"mimeType"` // an MCP image block's type
	ImageURL string `json:"imageUrl"` // a dynamic tool's data URL
}

// tool shows a tool call item as a tool item: its name, input, output, and
// images. A running call is kept until it completes.
func (ag *agent) tool(ev *harness.Event, parent string, it threadItem, completed bool) {
	item := harness.Item{ID: it.ID, Parent: parent, Kind: harness.KindTool}
	var input any
	switch it.Type {
	case "commandExecution":
		item.Tool, input = "shell", map[string]any{"command": it.Command, "cwd": it.Cwd}
		if it.Output != nil {
			item.Output = *it.Output
		}
		item.Failed = it.Status == "failed" || it.Status == "declined" || it.ExitCode != nil && *it.ExitCode != 0
	case "fileChange":
		paths := make([]string, len(it.Changes))
		for i, c := range it.Changes {
			paths[i] = c.Path
		}
		item.Tool, input = "apply_patch", map[string]any{"path": strings.Join(paths, ", "), "changes": it.Changes}
		item.Failed = it.Status == "failed" || it.Status == "declined"
	case "mcpToolCall":
		item.Server, item.Tool = it.Server, it.Tool
		item.Input = it.Arguments
		item.Failed = it.Status == "failed" || it.Error != nil
		switch {
		case it.Error != nil:
			item.Output = it.Error.Message
		case len(it.Result) > 0 && string(it.Result) != "null":
			var r toolResult
			if json.Unmarshal(it.Result, &r) == nil {
				item.Output = blocksText(r.Content)
				item.Images = blocksImages(r.Content)
				if item.Output == "" && len(item.Images) == 0 && len(r.StructuredContent) > 0 {
					item.Output = string(r.StructuredContent)
				}
			}
		}
	case "dynamicToolCall":
		item.Tool = it.Tool
		if it.Namespace != "" {
			item.Tool = it.Namespace + "__" + it.Tool
		}
		item.Input = it.Arguments
		item.Output = blocksText(it.ContentItems)
		item.Images = blocksImages(it.ContentItems)
		item.Failed = it.Status == "failed" || it.Success != nil && !*it.Success
	case "webSearch":
		item.Tool, input = "web_search", map[string]any{"query": it.Query}
	case "imageView":
		item.Tool, input = "view_image", map[string]any{"path": it.Path}
		if completed {
			if img, ok := fileImage(it.Path); ok {
				item.Images = []harness.Image{img}
			}
		}
	case "subAgentActivity":
		item.Tool, input = "subagent", map[string]any{"agent": it.AgentPath, "model": it.Model}
	case "collabAgentToolCall":
		item.Tool, input = "agent_"+it.Tool, map[string]any{"prompt": it.Prompt}
		item.Failed = it.Status == "failed"
	}
	if input != nil {
		var err error
		if item.Input, err = json.Marshal(input); err != nil {
			ag.fail(ev, "Codex sent a "+it.Type+" item that Hypercube cannot show: "+err.Error())
		}
	}
	if it.Status == "declined" && item.Output == "" {
		item.Output = "Declined."
	}
	if completed {
		ag.complete(ev, item)
		return
	}
	ag.running[it.ID] = item
	ev.Items = append(ev.Items, item)
}

// blocksText joins the text of content blocks.
func blocksText(blocks []contentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// blocksImages are the inline images among content blocks.
func blocksImages(blocks []contentBlock) []harness.Image {
	var images []harness.Image
	for _, b := range blocks {
		if img, ok := b.image(); ok {
			images = append(images, img)
		}
	}
	return images
}
