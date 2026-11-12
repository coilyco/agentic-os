use std::fmt;

use serde::{Deserialize, Serialize};

/// Kotlin's `takeReturn` answer. The address carries a one-use code, so `Debug` hides it.
#[derive(Clone, Default, PartialEq, Eq, Deserialize, Serialize)]
pub struct Pending {
  pub url: Option<String>,
}

/// The `return` event's payload. `Debug` hides the address for the same reason.
#[derive(Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Returned {
  pub url: String,
}

impl fmt::Debug for Pending {
  fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
    f.debug_struct("Pending").field("url", &self.url.as_ref().map(|_| "<hidden>")).finish()
  }
}

impl fmt::Debug for Returned {
  fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
    f.debug_struct("Returned").field("url", &"<hidden>").finish()
  }
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn the_return_address_never_reaches_a_debug_line() {
    let url = "dev.coilyco.treehouse:/signin?code=abc123&state=xyz";
    let shown = format!("{:?} {:?}", Returned { url: url.into() }, Pending { url: Some(url.into()) });
    assert!(!shown.contains("abc123") && !shown.contains("signin"), "{shown}");
  }

  #[test]
  fn kotlin_answers_decode_with_and_without_a_pending_return() {
    let waiting: Pending = serde_json::from_value(serde_json::json!({"url": "dev.coilyco.treehouse:/signin?code=c"})).unwrap();
    assert_eq!(waiting.url.as_deref(), Some("dev.coilyco.treehouse:/signin?code=c"));
    let none: Pending = serde_json::from_value(serde_json::json!({"url": null})).unwrap();
    assert_eq!(none, Pending::default());
  }
}
