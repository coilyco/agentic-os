use serde::Serialize;

/// The closed set of rejection reasons, the same strings the Kotlin side sends.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Code {
  Cancelled,
  Lockout,
  NoneEnrolled,
  Unsupported,
  Invalidated,
  Other,
}

impl Code {
  /// An unknown string is `other`, so a new Kotlin code cannot escape the closed set.
  pub fn parse(code: Option<&str>) -> Self {
    match code {
      Some("cancelled") => Self::Cancelled,
      Some("lockout") => Self::Lockout,
      Some("none_enrolled") => Self::NoneEnrolled,
      Some("unsupported") => Self::Unsupported,
      Some("invalidated") => Self::Invalidated,
      _ => Self::Other,
    }
  }
}

pub type Result<T> = std::result::Result<T, Error>;

/// Reaches the page as the rejection value `{code, message}`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, thiserror::Error)]
#[error("{code:?}: {message}")]
pub struct Error {
  pub code: Code,
  pub message: String,
}

impl Error {
  pub fn new(code: Code, message: impl Into<String>) -> Self {
    Self { code, message: message.into() }
  }
}

#[cfg(mobile)]
impl From<tauri::plugin::mobile::PluginInvokeError> for Error {
  fn from(error: tauri::plugin::mobile::PluginInvokeError) -> Self {
    use tauri::plugin::mobile::PluginInvokeError::InvokeRejected;
    match error {
      InvokeRejected(rejected) => Self::new(
        Code::parse(rejected.code.as_deref()),
        rejected.message.unwrap_or_default(),
      ),
      other => Self::new(Code::Other, other.to_string()),
    }
  }
}


#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn rejection_is_a_code_and_message_object() {
    let error = Error::new(Code::NoneEnrolled, "no screen lock");
    assert_eq!(
      serde_json::to_value(&error).unwrap(),
      serde_json::json!({"code": "none_enrolled", "message": "no screen lock"})
    );
  }

  #[test]
  fn every_code_the_page_switches_on_round_trips() {
    for name in ["cancelled", "lockout", "none_enrolled", "unsupported", "invalidated", "other"] {
      let wire = serde_json::to_value(Code::parse(Some(name))).unwrap();
      assert_eq!(wire, serde_json::json!(name));
    }
  }

  #[test]
  fn an_unknown_or_missing_code_is_other() {
    assert_eq!(Code::parse(Some("timeout")), Code::Other);
    assert_eq!(Code::parse(None), Code::Other);
  }
}
