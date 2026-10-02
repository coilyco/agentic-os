package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fakeAsker(answers ...choiceAnswer) (func(choiceAsk) (choiceAnswer, error), *[]string) {
	var asked []string
	index := 0
	return func(ask choiceAsk) (choiceAnswer, error) {
		asked = append(asked, ask.Question)
		answer := answers[index]
		index++
		return answer, nil
	}, &asked
}

func picked(label string) choiceAnswer {
	return choiceAnswer{State: "answered", Picks: []int{0}, Labels: []string{label}}
}

const oneQuestion = `{"question":"Ship it?","options":[{"label":"yes"},{"label":"no"}]}`

func TestAskChoiceSingleQuestionKeepsItsOldShape(t *testing.T) {
	ask, asked := fakeAsker(picked("yes"))
	out, failed := askFromArguments(json.RawMessage(oneQuestion), ask)
	if failed || out != `{"labels":["yes"],"picks":[0],"text":""}` || len(*asked) != 1 {
		t.Fatalf("got %q failed=%v asked=%v", out, failed, *asked)
	}
}

func TestAskChoiceTakesUpToFourQuestionsAndReturnsAnAnswerEach(t *testing.T) {
	ask, asked := fakeAsker(picked("a"), picked("b"))
	out, failed := askFromArguments(json.RawMessage(`{"questions":[
		{"question":"First?","header":"One","options":[{"label":"a"}]},
		{"question":"Second?","options":[{"label":"b"}],"multi":true}]}`), ask)
	if failed || strings.Join(*asked, "|") != "First?|Second?" {
		t.Fatalf("got %q failed=%v asked=%v", out, failed, *asked)
	}
	var decoded struct {
		Answers []struct {
			Question string   `json:"question"`
			Header   string   `json:"header"`
			Labels   []string `json:"labels"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil || len(decoded.Answers) != 2 ||
		decoded.Answers[0].Header != "One" || decoded.Answers[1].Labels[0] != "b" {
		t.Fatalf("answers: %q %v", out, err)
	}
}

func TestAskChoiceRefusesBeforeShowingAnyCard(t *testing.T) {
	cases := map[string]string{
		"both shapes":     `{"question":"x","options":[{"label":"a"}],"questions":[{"question":"y","options":[{"label":"b"}]}]}`,
		"five questions":  `{"questions":[` + strings.Repeat(`{"question":"q","options":[{"label":"a"}]},`, 4) + `{"question":"q","options":[{"label":"a"}]}]}`,
		"a later invalid": `{"questions":[{"question":"ok","options":[{"label":"a"}]},{"question":"bad"}]}`,
	}
	for name, arguments := range cases {
		ask, asked := fakeAsker(picked("a"), picked("a"))
		if out, failed := askFromArguments(json.RawMessage(arguments), ask); !failed || len(*asked) != 0 {
			t.Errorf("%s: failed=%v asked=%v out=%q", name, failed, *asked, out)
		}
	}
}

func TestAskChoiceKeepsEarlierAnswersWhenALaterOneIsCancelled(t *testing.T) {
	ask, _ := fakeAsker(picked("a"), choiceAnswer{State: "cancelled", Reason: "a client dismissed it"})
	out, failed := askFromArguments(json.RawMessage(`{"questions":[
		{"question":"One?","options":[{"label":"a"}]},{"question":"Two?","options":[{"label":"b"}]}]}`), ask)
	if !failed || !strings.Contains(out, "question 2 of 2: the ask was cancelled: a client dismissed it") ||
		!strings.Contains(out, `"labels":["a"]`) {
		t.Fatalf("got %q failed=%v", out, failed)
	}
}

func TestAskChoiceReportsATransportErrorWithProgress(t *testing.T) {
	calls := 0
	ask := func(choiceAsk) (choiceAnswer, error) {
		calls++
		if calls == 2 {
			return choiceAnswer{}, errors.New("the daemon went away")
		}
		return picked("a"), nil
	}
	out, failed := askFromArguments(json.RawMessage(`{"questions":[
		{"question":"One?","options":[{"label":"a"}]},{"question":"Two?","options":[{"label":"b"}]}]}`), ask)
	if !failed || !strings.HasPrefix(out, "the daemon went away. Answered before that:") {
		t.Fatalf("got %q failed=%v", out, failed)
	}
}
