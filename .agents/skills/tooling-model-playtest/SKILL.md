---
name: tooling-model-playtest
description: Playtest a game by having models play it, so their reactions drive the design. Covers keeping the prompt deliberately vague, splitting a decision model from a freeform one, and reading the result. Triggers - model playtest, AI playtester, let a model play it, playtest with an LLM, does the UI communicate, game feedback from a model, is my game legible, agent plays my game, playtest harness.
---

# Let the game be judged by something that did not build it

A model playtest exists to change the game. The playtester's reactions are the
deliverable, so every instinct to help it play well is destroying the thing you
came for. The prompt stays as thin as it can be and the findings come out of what
the playtester does and says.

**Why:** the first version of one of these handed the model the tuning constants
and the objective, glossed each option with the author's own strategy, and then
scored the freeform answer into fixed rubrics. It produced a confident report
that was mostly a readback of the prompt. Removing the glosses alone dropped the
same loop from six placements to one, which means the glosses had been playing
the game, not the playtester.

## Two playtesters, because they answer different questions

* **A decision model** - returns a probability for every option it was offered.
  It measures whether the visible state supports a decision at all. Its
  distribution is the instrument, not its chosen move.
* **A freeform model** - answers in prose. It says what playing was like, and
  what it believed the rules were. Keep its words whole.

Run both. Where they disagree, you have learned that a behaviour is one model's
habit rather than a property of the game.

**Why:** a single decision model declined to act on almost every turn, which read
as proof the turn was degenerate. A chat model on the identical state acted every
turn. The first conclusion was about the model.

## Prepare the thinnest prompt that can still be played

1. Send the title and the visible state. Nothing else.
2. Withhold the objective. Whether the objective is discoverable is one of the
   things being measured, and stating it erases that measurement.
3. Withhold every tuning number. Rates, thresholds, spans, radii, stage counts.
   A playtester that is told the rules cannot invent a wrong set, and the wrong
   set is the finding.
4. Describe each action as what it does and where, with no adjective. A word like
   "safer", or a phrase naming what the action is good for, is the author playing
   through the playtester.
5. Send no derived salience. No distances, adjacency counts, rankings, or
   highlights that the interface does not itself show. Ordering the options is a
   judgement, so order them on something neutral such as proximity.
6. Ask one question, and let it name no axis. "Should you keep expanding" already
   decided that expansion is the question.
7. Keep a guard test that fails when a banned word reappears in the prompt. The
   pressure to add one arrives every time a session disappoints.

## Run

1. Offer only actions the server would accept, and build the offer from the state
   at the moment the move is played. A standing order can carry the player out of
   range between the offer and the command landing, and the refusal is silent.
2. Refuse an answer naming an option that was never offered. Never substitute a
   default move, which manufactures play that no model chose.
3. Write a transcript and flush it every turn. It holds each prompt, each reply,
   and every word said. A session that dies partway still leaves its evidence,
   and sessions do die partway.
4. Use a fresh world per session and take several. One session is an anecdote.
5. Constrain the reply only where a move has to parse. Leave the freeform answer
   unconstrained, with no schema, no scale, and no list of things to comment on.

## Reasoning models need room, not restraint

* A reasoning model's thinking shares the output budget with its answer. Too
  small a budget returns an empty answer with a length finish reason: it thought
  until the cap and never replied. Read that as a budget error, not a refusal.
* Do not throttle the reasoning. Measured on one turn, a throttled setting cost
  10,888 completion tokens in 49 seconds against 6,900 in 33 seconds unthrottled,
  and it truncated the thinking that is the material worth reading.
* Check what the gateway actually caps before picking a number. A hosted route
  often imposes nothing and forwards the provider's own limit, so a low cap is
  usually the author's guess rather than a constraint.
* Its reasoning grows with the state, so a budget that carried the first turn can
  run out later. Allow one retry at a larger budget and fail loudly after that.

## Read the result

* **Mass spread evenly** - the visible state does not distinguish the options.
  The interface is not telling a player enough to choose.
* **One option dominating "do nothing"** - there is a reason to act and it lands.
  The reverse, doing nothing winning, says the game offers no reason to act.
* **A cluster at or near zero** - those options are indistinguishable noise. If
  they are meaningfully different in the design, that difference is invisible.
* **Rules in the prose that do not exist** - the highest value finding available.
  An interface that reads as a quest, a counter, or an instruction will have a
  player inventing mechanics and acting on them. Quote it and fix the surface.
* **What it never tried** - an action class that never appears is one the game
  gives no reason to use.

## Never

* Never tune the prompt because a session went badly. Fix the game instead.
* Never score a freeform answer into a rubric. A rubric decides in advance what
  the playtest may find, and one asked without evidence returns a number at
  almost zero confidence that still reads like a verdict.
* Never summarise the session back in your own words when asking for feedback.
  Show it what it actually did, turn by turn, because the experience being asked
  about is its own.
* Never report what you changed in the prompt as a finding. The findings are what
  the game failed to convey.

## Hand the findings over

Each finding is a question for whoever owns the design, phrased as what the game
must convey rather than as a fix you chose. File them where decisions get made,
with the quote or the distribution that produced them. A playtest whose output
lives only in a terminal did not happen.
