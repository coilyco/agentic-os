use serde::{Deserialize, Serialize};

/// `available`. Whether a speech engine started, which one, and how many voices it lists.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Availability {
  pub available: bool,
  pub engine: Option<String>,
  pub voices: u32,
}

/// `speak`. The reply's own id, so the page knows which reply a `poll` is about.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Accepted {
  pub id: String,
}

/// The engine's phase. `poll` reports it, and the page decides what each means.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Phase {
  Idle,
  Speaking,
  Done,
  /// `error` holds Android's `TextToSpeech.ERROR_*` code.
  Error,
}

/// `poll`. `started_ms` and `done_ms` count from `speak`. `seq` rises on every change.
/// Design: references/android-speak.md.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Spoken {
  pub seq: u32,
  pub phase: Phase,
  pub id: Option<String>,
  #[serde(rename = "startedMs")]
  pub started_ms: Option<u64>,
  #[serde(rename = "doneMs")]
  pub done_ms: Option<u64>,
  pub error: Option<i32>,
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn kotlin_answer_decodes() {
    let wire = serde_json::json!({"seq": 4, "phase": "done", "id": "r7", "startedMs": 310, "doneMs": 2480, "error": null});
    let spoken: Spoken = serde_json::from_value(wire).unwrap();
    assert_eq!(spoken.phase, Phase::Done);
    assert_eq!(spoken.id.as_deref(), Some("r7"));
    assert_eq!((spoken.started_ms, spoken.done_ms), (Some(310), Some(2480)));
  }

  #[test]
  fn an_idle_answer_has_no_id_or_times() {
    let wire = serde_json::json!({"seq": 0, "phase": "idle", "id": null, "startedMs": null, "doneMs": null, "error": null});
    let spoken: Spoken = serde_json::from_value(wire).unwrap();
    assert_eq!((spoken.id, spoken.started_ms, spoken.done_ms), (None, None, None));
  }

  #[test]
  fn every_phase_the_page_switches_on_round_trips() {
    for name in ["idle", "speaking", "done", "error"] {
      let phase: Phase = serde_json::from_value(serde_json::json!(name)).unwrap();
      assert_eq!(serde_json::to_value(phase).unwrap(), serde_json::json!(name));
    }
  }

  #[test]
  fn the_availability_answer_decodes() {
    let wire = serde_json::json!({"available": true, "engine": "com.google.android.tts", "voices": 214});
    let seen: Availability = serde_json::from_value(wire).unwrap();
    assert!(seen.available);
    assert_eq!((seen.engine.as_deref(), seen.voices), (Some("com.google.android.tts"), 214));
  }

  #[test]
  fn the_page_reads_started_ms_and_done_ms_by_those_names() {
    let spoken = Spoken { seq: 1, phase: Phase::Speaking, id: Some("a".into()), started_ms: Some(5), done_ms: None, error: None };
    let wire = serde_json::to_value(spoken).unwrap();
    assert_eq!(wire["startedMs"], 5);
    assert!(wire.get("started_ms").is_none());
  }
}
