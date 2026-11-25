use std::fmt;

use serde::{Deserialize, Serialize};

/// `register`'s answer: the body of treehouse.push.app-register.v1 less `platform`.
/// The token addresses this phone, so `Debug` hides it.
#[derive(Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Registered {
  pub token: String,
  pub device: String,
}

/// Kotlin's `token` answer, null before the first `register`.
#[cfg_attr(desktop, allow(dead_code))]
#[derive(Clone, Default, PartialEq, Eq, Deserialize, Serialize)]
pub(crate) struct Token {
  pub token: Option<String>,
}

/// A tapped push: the seat (`session`) or, for a `reachable` push, only the machine.
/// It carries ids and no text, so `Debug` shows it.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize, Serialize)]
pub struct Tap {
  pub kind: String,
  pub daemon_id: String,
  pub session: Option<String>,
  pub tag: String,
}

/// Kotlin's `takeTap` answer.
#[cfg_attr(desktop, allow(dead_code))]
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize, Serialize)]
pub(crate) struct Taken {
  pub tap: Option<Tap>,
}

impl fmt::Debug for Registered {
  fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
    f.debug_struct("Registered").field("token", &"<hidden>").field("device", &self.device).finish()
  }
}

impl fmt::Debug for Token {
  fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
    f.debug_struct("Token").field("token", &self.token.as_ref().map(|_| "<hidden>")).finish()
  }
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn the_token_never_reaches_a_debug_line() {
    let token = "fGc0:APA91bSecretTokenValue";
    let shown = format!(
      "{:?} {:?}",
      Registered { token: token.into(), device: "Pixel 8".into() },
      Token { token: Some(token.into()) }
    );
    assert!(!shown.contains("APA91b") && shown.contains("Pixel 8"), "{shown}");
  }

  #[test]
  fn kotlin_answers_decode() {
    let registered: Registered = serde_json::from_value(serde_json::json!({"token": "t:1", "device": "Pixel 8"})).unwrap();
    assert_eq!(registered.token, "t:1");
    let none: Token = serde_json::from_value(serde_json::json!({"token": null})).unwrap();
    assert_eq!(none, Token::default());
  }

  #[test]
  fn a_seat_tap_and_a_machine_tap_decode() {
    let seat: Taken = serde_json::from_value(serde_json::json!({"tap": {
      "kind": "ask", "daemon_id": "d1", "session": "eng-platform-tender-beetle", "tag": "aterm-eng-platform-tender-beetle"
    }}))
    .unwrap();
    assert_eq!(seat.tap.unwrap().session.as_deref(), Some("eng-platform-tender-beetle"));
    let machine: Taken = serde_json::from_value(serde_json::json!({"tap": {
      "kind": "reachable", "daemon_id": "d1", "session": null, "tag": "treehouse-reachable-d1"
    }}))
    .unwrap();
    assert_eq!(machine.tap.unwrap().session, None);
    let nothing: Taken = serde_json::from_value(serde_json::json!({"tap": null})).unwrap();
    assert_eq!(nothing, Taken::default());
  }
}
