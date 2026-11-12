use serde::Serialize;

/// Reaches the page as the rejection value `{code, message}`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, thiserror::Error)]
#[error("{code}: {message}")]
pub struct Error {
  pub code: String,
  pub message: String,
}

pub type Result<T> = std::result::Result<T, Error>;

impl Error {
  /// An address the plugin will not open. `message` is fixed and never echoes it.
  pub fn refused(message: &str) -> Self {
    Self { code: "refused".into(), message: message.into() }
  }

  #[cfg(not(any(target_os = "macos", mobile)))]
  pub fn unsupported() -> Self {
    Self { code: "unsupported".into(), message: "this platform has no system browser session".into() }
  }
}

impl From<tauri::Error> for Error {
  fn from(error: tauri::Error) -> Self {
    Self { code: "other".into(), message: error.to_string() }
  }
}

#[cfg(mobile)]
impl From<tauri::plugin::mobile::PluginInvokeError> for Error {
  fn from(error: tauri::plugin::mobile::PluginInvokeError) -> Self {
    use tauri::plugin::mobile::PluginInvokeError::InvokeRejected;
    match error {
      InvokeRejected(rejected) => Self {
        code: rejected.code.unwrap_or_else(|| "other".into()),
        message: rejected.message.unwrap_or_default(),
      },
      other => Self { code: "other".into(), message: other.to_string() },
    }
  }
}
