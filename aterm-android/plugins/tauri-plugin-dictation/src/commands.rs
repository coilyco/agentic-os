use tauri::{AppHandle, Runtime, command};

use crate::DictationExt;
use crate::error::Result;
use crate::models::{Availability, Heard};

// `start` waits on the microphone prompt, so every command runs off the main thread.
#[command]
pub(crate) async fn available<R: Runtime>(app: AppHandle<R>) -> Result<Availability> {
  app.dictation().available()
}

#[command]
pub(crate) async fn start<R: Runtime>(app: AppHandle<R>, language: Option<String>) -> Result<()> {
  app.dictation().start(language)
}

#[command]
pub(crate) async fn poll<R: Runtime>(app: AppHandle<R>) -> Result<Heard> {
  app.dictation().poll()
}

#[command]
pub(crate) async fn stop<R: Runtime>(app: AppHandle<R>) -> Result<()> {
  app.dictation().stop()
}

#[command]
pub(crate) async fn cancel<R: Runtime>(app: AppHandle<R>) -> Result<()> {
  app.dictation().cancel()
}
