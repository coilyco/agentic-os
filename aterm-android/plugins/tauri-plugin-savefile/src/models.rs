use serde::{Deserialize, Serialize};

/// `available`. Lets the page tell this build from an older app without the plugin.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Available {
  /// Where a saved file lands, in words the page can show.
  pub place: String,
}

/// `begin`. The name the file was given, which may differ from the one asked for.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Begun {
  pub id: String,
  pub name: String,
}

/// `finish`. What the person is told: the saved name and the place it went.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Saved {
  pub name: String,
  pub place: String,
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn the_answers_carry_the_keys_the_page_reads() {
    let begun = serde_json::to_value(Begun { id: "1".into(), name: "a.txt".into() }).unwrap();
    assert_eq!(begun, serde_json::json!({"id": "1", "name": "a.txt"}));
    let saved = serde_json::to_value(Saved { name: "a.txt".into(), place: "Downloads".into() }).unwrap();
    assert_eq!(saved, serde_json::json!({"name": "a.txt", "place": "Downloads"}));
  }
}
