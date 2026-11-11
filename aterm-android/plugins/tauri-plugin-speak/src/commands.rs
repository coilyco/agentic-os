use tauri::{AppHandle, Runtime, command};

use crate::SpeakExt;
use crate::error::Result;
use crate::models::{Accepted, Availability, Spoken};

// `available` and `speak` wait on the engine, so commands run off the main thread.
#[command]
pub(crate) async fn available<R: Runtime>(app: AppHandle<R>) -> Result<Availability> {
  app.speak().available()
}

#[command]
pub(crate) async fn speak<R: Runtime>(
  app: AppHandle<R>,
  id: String,
  text: String,
  language: Option<String>,
  rate: Option<f32>,
) -> Result<Accepted> {
  app.speak().speak(id, text, language, rate)
}

#[command]
pub(crate) async fn stop<R: Runtime>(app: AppHandle<R>) -> Result<()> {
  app.speak().stop()
}

#[command]
pub(crate) async fn poll<R: Runtime>(app: AppHandle<R>) -> Result<Spoken> {
  app.speak().poll()
}
