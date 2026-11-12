use tauri::{AppHandle, Runtime, command};

use crate::SystemBrowserExt;
use crate::error::Result;

// Async, so starting a desktop session on the main thread never waits on itself.
#[command]
pub(crate) async fn open<R: Runtime>(app: AppHandle<R>, url: String) -> Result<()> {
  app.systembrowser().open(&url)
}

#[command]
pub(crate) async fn take_return<R: Runtime>(app: AppHandle<R>) -> Result<Option<String>> {
  app.systembrowser().take_return()
}

#[cfg(desktop)]
#[command]
pub(crate) fn register_listener<R: Runtime>(
  webview: tauri::Webview<R>,
  app: AppHandle<R>,
  event: String,
  handler: tauri::ipc::Channel<serde_json::Value>,
) {
  app.systembrowser().register_listener(event, handler, webview.label());
}

#[cfg(desktop)]
#[command]
pub(crate) fn remove_listener<R: Runtime>(app: AppHandle<R>, event: String, channel_id: u32) {
  app.systembrowser().remove_listener(&event, channel_id);
}
