use serde::Serialize;

/// Reaches the page as the rejection value `{code, message}`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, thiserror::Error)]
#[error("{code}: {message}")]
pub struct Error {
  pub code: String,
  pub message: String,
}

pub type Result<T> = std::result::Result<T, Error>;

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
