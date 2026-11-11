use serde::{Deserialize, Serialize};

/// `available`. Whether a speech recognizer exists, and the microphone grant.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Availability {
  pub available: bool,
  /// `granted`, `denied` or `prompt`. Only `prompt` can still ask.
  pub permission: String,
}

/// The recognizer's phase. `poll` reports it, and the page decides what each means.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Phase {
  Idle,
  Listening,
  /// Stopped, with a final transcript waiting in `text`.
  Done,
  /// Stopped, and `error` holds Android's `SpeechRecognizer.ERROR_*` code.
  Failed,
}

/// `poll`. `text` is the words so far, final once done. `seq` rises on every change.
/// Design: references/android-dictation.md.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Heard {
  pub phase: Phase,
  pub text: String,
  pub error: Option<i32>,
  pub seq: u32,
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn kotlin_answer_decodes() {
    let wire = serde_json::json!({"phase": "listening", "text": "hello wor", "error": null, "seq": 3});
    let heard: Heard = serde_json::from_value(wire).unwrap();
    assert_eq!(heard.phase, Phase::Listening);
    assert_eq!(heard.text, "hello wor");
    assert_eq!(heard.seq, 3);
  }

  #[test]
  fn every_phase_the_page_switches_on_round_trips() {
    for name in ["idle", "listening", "done", "failed"] {
      let phase: Phase = serde_json::from_value(serde_json::json!(name)).unwrap();
      assert_eq!(serde_json::to_value(phase).unwrap(), serde_json::json!(name));
    }
  }

  #[test]
  fn the_availability_answer_decodes() {
    let wire = serde_json::json!({"available": true, "permission": "prompt"});
    let seen: Availability = serde_json::from_value(wire).unwrap();
    assert!(seen.available);
    assert_eq!(seen.permission, "prompt");
  }
}
