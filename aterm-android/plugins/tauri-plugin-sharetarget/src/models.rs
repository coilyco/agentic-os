use serde::{Deserialize, Serialize};

/// One shared image. `data` is base64 of the bytes, `mime` is Android's type.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct SharedImage {
  pub name: String,
  pub mime: String,
  pub data: String,
}

/// Why one shared item was left out. A closed set, so the page can say each in words.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Reason {
  /// Not an image type, or no type at all.
  NotImage,
  /// Over the plugin's read cap, so it was not loaded.
  TooBig,
  /// The sender's grant ended or the file is gone.
  Unreadable,
  /// More items than one message carries.
  TooMany,
}

#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Skipped {
  pub name: String,
  pub reason: Reason,
}

/// `take`. Everything the Share menu handed the app since the last take, then forgotten.
/// `text` is words the sender shared with the picture, for the words box.
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize, Serialize)]
pub struct Shared {
  pub images: Vec<SharedImage>,
  pub skipped: Vec<Skipped>,
  pub text: Option<String>,
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn the_empty_answer_is_what_a_page_with_nothing_waiting_gets() {
    assert_eq!(
      serde_json::to_value(Shared::default()).unwrap(),
      serde_json::json!({"images": [], "skipped": [], "text": null})
    );
  }

  #[test]
  fn every_reason_the_page_switches_on_round_trips() {
    for name in ["not_image", "too_big", "unreadable", "too_many"] {
      let reason: Reason = serde_json::from_value(serde_json::json!(name)).unwrap();
      assert_eq!(serde_json::to_value(reason).unwrap(), serde_json::json!(name));
    }
  }

  #[test]
  fn kotlin_answer_decodes() {
    let wire = serde_json::json!({
      "images": [{"name": "Screenshot_1.png", "mime": "image/png", "data": "AAEC"}],
      "skipped": [{"name": "clip.mp4", "reason": "not_image"}],
      "text": "look at this"
    });
    let shared: Shared = serde_json::from_value(wire).unwrap();
    assert_eq!(shared.images[0].name, "Screenshot_1.png");
    assert_eq!(shared.skipped[0].reason, Reason::NotImage);
    assert_eq!(shared.text.as_deref(), Some("look at this"));
  }
}
