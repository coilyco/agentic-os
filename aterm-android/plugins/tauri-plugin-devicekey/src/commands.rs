use tauri::{AppHandle, Runtime, command};

use crate::DeviceKeyExt;
use crate::error::Result;
use crate::models::{Enrolled, Signed, Status};

// Each command blocks until the person answers, so it runs off the main thread.
#[command]
pub(crate) async fn status<R: Runtime>(app: AppHandle<R>) -> Result<Status> {
  app.devicekey().status()
}

#[command]
pub(crate) async fn enroll<R: Runtime>(app: AppHandle<R>, challenge: String) -> Result<Enrolled> {
  app.devicekey().enroll(challenge)
}

#[command]
pub(crate) async fn sign<R: Runtime>(app: AppHandle<R>, challenge: String, prompt: Option<String>) -> Result<Signed> {
  app.devicekey().sign(challenge, prompt)
}

#[command]
pub(crate) async fn forget<R: Runtime>(app: AppHandle<R>) -> Result<()> {
  app.devicekey().forget()
}
