use tauri::{AppHandle, Runtime, command};

use crate::PushExt;
use crate::error::Result;
use crate::models::{Registered, Tap};

// `register` waits on the push service, so every command runs off the main thread.
#[command]
pub(crate) async fn register<R: Runtime>(app: AppHandle<R>) -> Result<Registered> {
  app.push().register()
}

#[command]
pub(crate) async fn token<R: Runtime>(app: AppHandle<R>) -> Result<Option<String>> {
  app.push().token()
}

#[command]
pub(crate) async fn take_tap<R: Runtime>(app: AppHandle<R>) -> Result<Option<Tap>> {
  app.push().take_tap()
}
