package main

import (
	"testing"
)

func TestMultipleQuestionSelectionsReachClaude(t *testing.T) {
	h := newClaude()
	h.feed(`{"type":"control_request","request_id":"multi","request":{"subtype":"can_use_tool","tool_name":"AskUserQuestion","input":{"questions":[{"question":"Which colors?","multiSelect":true,"options":[{"label":"Red"},{"label":"Blue"}]}]}}}`)
	item := h.transcript()[0]
	if !item.Multi {
		t.Fatal("the question lost its multiple-selection setting")
	}
	if err := h.answer(item.ID, []string{"Red", "Blue"}, "Green"); err != nil {
		t.Fatal(err)
	}
	_, body := h.reply(t)
	input := body["updatedInput"].(map[string]any)
	answers := input["answers"].(map[string]any)
	if answers["Which colors?"] != "Red, Blue, Green" {
		t.Fatalf("Claude answers = %v", answers)
	}
	if item := h.transcript()[0]; !item.Done || item.Answer != "Red, Blue, Green" || !item.Multi {
		t.Fatalf("answered item = %+v", item)
	}
}
