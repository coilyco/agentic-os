use serde::Serialize;

/// Reaches the page as the rejection value `{code, message}`. `push_off` means this build
/// or this device has no push, which the page treats as a setting, not a fault.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, thiserror::Error)]
#[error("{code}: {message}")]
pub struct Error {
  pub code: String,
  pub message: String,
}

pub type Result<T> = std::result::Result<T, Error>;

impl Error {
  #[cfg(desktop)]
  pub fn push_off(message: &str) -> Self {
    Self { code: "push_off".into(), message: message.into() }
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
