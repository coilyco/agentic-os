use tauri::{AppHandle, Runtime, command};

use crate::ShareTargetExt;
use crate::error::Result;
use crate::models::Shared;

// The Kotlin side waits for the files to be read, so this runs off the main thread.
#[command]
pub(crate) async fn take<R: Runtime>(app: AppHandle<R>) -> Result<Shared> {
  app.sharetarget().take()
}
