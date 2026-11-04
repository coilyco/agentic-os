package main

import (
	"strings"
	"testing"
	"time"
)

func TestPickDirectorPrefersTheOneAClientHasOpenThenTheOneTypedIntoLast(t *testing.T) {
	old, recent := time.Now().Add(-time.Hour), time.Now()
	cases := []struct {
		name string
		in   []directorCandidate
		want string
	}{
		{"none up", nil, ""},
		{"a client has it open", []directorCandidate{{"a", 0, recent, old}, {"b", 1, old, recent}}, "b"},
		{"typed into last", []directorCandidate{{"a", 0, old, old}, {"b", 0, recent, recent}}, "b"},
		{"oldest when the rest ties", []directorCandidate{{"b", 0, time.Time{}, recent}, {"a", 0, time.Time{}, old}}, "a"},
		{"name settles a full tie", []directorCandidate{{"b", 0, time.Time{}, old}, {"a", 0, time.Time{}, old}}, "a"},
	}
	for _, c := range cases {
		if got := pickDirector(c.in); got != c.want {
			t.Errorf("%s: picked %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRelayTextNamesTheAskerTheQuestionAndHowItEnded(t *testing.T) {
	ask := choiceAsk{From: "eng-platform Beetle-Ox", Question: "Ship it?\nNow."}
	got := relayText(ask, choiceAnswer{State: "answered", Labels: []string{"yes"}, Text: "after lunch"})
	if want := `Kai answered eng-platform Beetle-Ox's question "Ship it? Now.": yes, typed: after lunch`; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	got = relayText(ask, choiceAnswer{State: "timed_out"})
	if want := `eng-platform Beetle-Ox's question "Ship it? Now." ended unanswered: timed out`; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}

// spawnDirector is a live prod-director seat, up for as long as its script runs.
func spawnDirector(t *testing.T, script string) *testClient {
	t.Helper()
	director := dialTest(t)
	director.spawn("prod-director-griffin-goose", "prod-director", "Griffin-Goose", script)
	director.until("READY")
	return director
}

func TestSendUserQuestionLandsOnTheDirectorAndTheAnswerGoesBack(t *testing.T) {
	_, token := askingSeat(t)
	director := spawnDirector(t, pasteCat)
	client := subscribed(t)
	ask := colors
	ask.Relay = true
	ask.CardSeat = "forged-seat" // the daemon stamps this, a caller cannot choose it
	_, replies := startAsk(t, token, ask)
	shown := nextFrame(t, client, "ask").Ask
	if shown.Session != "eng-platform-beetle-ox" || shown.CardSeat != "prod-director-griffin-goose" || !shown.Relay {
		t.Fatalf("the card should land on the director with the worker still the asker: %+v", shown)
	}
	if err := client.write(frame{Type: "answer", ID: "a", AskID: shown.ID, Picks: []int{1}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	reply := <-replies
	if reply.Type != "answered" || reply.Answer.CardSeat != "prod-director-griffin-goose" || strings.Join(reply.Answer.Labels, ",") != "green" {
		t.Fatalf("the worker should get the pick back, told where the card was: %+v %+v", reply, reply.Answer)
	}
	director.until(`[from aterm daemon] Kai answered eng-platform Beetle-Ox's question "Which color?": green`)
}

func TestAnAskWithNoDirectorUpShowsOnTheAskerAndNoCallerChoosesACardSeat(t *testing.T) {
	_, token := askingSeat(t)
	client := subscribed(t)
	ask := colors
	ask.Relay, ask.CardSeat = true, "forged-seat"
	_, replies := startAsk(t, token, ask)
	shown := nextFrame(t, client, "ask").Ask
	if shown.CardSeat != "" || shown.Session != "eng-platform-beetle-ox" {
		t.Fatalf("with no director the card shows on the asker, unrelayed: %+v", shown)
	}
	if err := client.write(frame{Type: "answer", ID: "a", AskID: shown.ID, Picks: []int{0}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if reply := <-replies; reply.Answer == nil || reply.Answer.State != "answered" || reply.Answer.CardSeat != "" {
		t.Fatalf("the worker should still be answered: %+v", reply)
	}
}

func TestAPlainAskIsNeverRelayedEvenWithADirectorUp(t *testing.T) {
	_, token := askingSeat(t)
	spawnDirector(t, pasteCat)
	client := subscribed(t)
	ask := colors
	ask.CardSeat = "prod-director-griffin-goose"
	startAsk(t, token, ask)
	if shown := nextFrame(t, client, "ask").Ask; shown.CardSeat != "" || shown.Relay {
		t.Fatalf("ask_choice keeps its card on the asker: %+v", shown)
	}
}

func TestTheCardMovesOffADirectorThatEndsWhileTheQuestionIsPending(t *testing.T) {
	_, token := askingSeat(t)
	spawnDirector(t, `printf 'READY\033[?2004h\n'; sleep 3`)
	client := subscribed(t)
	ask := colors
	ask.Relay = true
	_, replies := startAsk(t, token, ask)
	first := nextFrame(t, client, "ask").Ask
	if first.CardSeat != "prod-director-griffin-goose" {
		t.Fatalf("the card should start on the director: %+v", first)
	}
	moved := nextFrame(t, client, "ask").Ask
	if moved.ID != first.ID || moved.CardSeat != "" {
		t.Fatalf("the same ask should come back with no card seat once the director ends: %+v", moved)
	}
	if err := client.write(frame{Type: "answer", ID: "a", AskID: first.ID, Picks: []int{0}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if reply := <-replies; reply.Answer == nil || reply.Answer.State != "answered" {
		t.Fatalf("the question should survive its director ending: %+v", reply)
	}
}
