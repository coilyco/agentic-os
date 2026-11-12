use std::sync::Arc;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::Result;
use crate::listeners::Listeners;
use crate::rules;

#[cfg(target_os = "macos")]
mod macos;

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<SystemBrowser<R>> {
  Ok(SystemBrowser { app: app.clone(), listeners: Arc::new(Listeners::default()) })
}

/// A Mac's authentication session. Its return arrives in the session's own completion
/// handler and never as a scheme open, so no bundle registration is involved.
pub struct SystemBrowser<R: Runtime> {
  app: AppHandle<R>,
  listeners: Arc<Listeners>,
}

impl<R: Runtime> SystemBrowser<R> {
  pub fn open(&self, raw: &str) -> Result<()> {
    let url = rules::check_open(raw)?;
    #[cfg(target_os = "macos")]
    return macos::start(&self.app, self.listeners.clone(), &url);
    #[cfg(not(target_os = "macos"))]
    {
      let _ = (&self.app, url);
      Err(crate::error::Error::unsupported())
    }
  }

  /// The session dies with the app, so nothing is ever waiting across a start.
  pub fn take_return(&self) -> Result<Option<String>> {
    Ok(None)
  }

  pub fn register_listener(&self, event: String, channel: tauri::ipc::Channel<serde_json::Value>, webview: &str) {
    self.listeners.register(event, channel, webview);
  }

  pub fn remove_listener(&self, event: &str, channel_id: u32) {
    self.listeners.remove(event, channel_id);
  }

  pub fn remove_webview_listeners(&self, webview: &str) {
    self.listeners.remove_webview(webview);
  }
}
