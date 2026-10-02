package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// maxQuestionsPerCall matches what Claude Code's own AskUserQuestion takes.
const maxQuestionsPerCall = 4

// askFromArguments runs ask_choice: one question in the top-level fields, or up to
// four in `questions`, asked one card at a time. docs/aterm-daemon.md
func askFromArguments(arguments json.RawMessage, ask func(choiceAsk) (choiceAnswer, error)) (string, bool) {
	var params struct {
		choiceAsk
		Questions []choiceAsk `json:"questions"`
	}
	if err := json.Unmarshal(arguments, &params); err != nil {
		return err.Error(), true
	}
	asks := params.Questions
	switch {
	case len(asks) > 0 && params.Question != "":
		return "give either question or questions, not both", true
	case len(asks) == 0:
		asks = []choiceAsk{params.choiceAsk}
	case len(asks) > maxQuestionsPerCall:
		return fmt.Sprintf("an ask takes at most %d questions", maxQuestionsPerCall), true
	}
	for index, one := range asks {
		if err := validateAsk(one); err != nil {
			return fmt.Sprintf("question %d: %v", index+1, err), true
		}
	}
	answers := make([]map[string]any, 0, len(asks))
	for index, one := range asks {
		answer, err := ask(one)
		if err != nil {
			return withProgress(err.Error(), answers), true
		}
		if answer.State != "answered" {
			reason := fmt.Sprintf("the ask was %s: %s", strings.ReplaceAll(answer.State, "_", " "), answer.Reason)
			if len(asks) > 1 {
				reason = fmt.Sprintf("question %d of %d: %s", index+1, len(asks), reason)
			}
			return withProgress(reason, answers), true
		}
		answers = append(answers, map[string]any{
			"question": one.Question, "header": one.Header,
			"picks": answer.Picks, "labels": answer.Labels, "text": answer.Text,
		})
	}
	if len(params.Questions) == 0 {
		encoded, _ := json.Marshal(map[string]any{"picks": answers[0]["picks"], "labels": answers[0]["labels"], "text": answers[0]["text"]})
		return string(encoded), false
	}
	encoded, _ := json.Marshal(map[string]any{"answers": answers})
	return string(encoded), false
}

// withProgress appends what was already answered to a failure, so a seat that
// asked four questions keeps the answers Kai gave before one was cancelled.
func withProgress(message string, answers []map[string]any) string {
	if len(answers) == 0 {
		return message
	}
	encoded, _ := json.Marshal(map[string]any{"answers": answers})
	return message + ". Answered before that: " + string(encoded)
}
