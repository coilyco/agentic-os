use tauri::{AppHandle, Runtime, command};

use crate::SaveFileExt;
use crate::error::Result;
use crate::models::{Available, Begun, Saved};

#[command]
pub(crate) async fn available<R: Runtime>(app: AppHandle<R>) -> Result<Available> {
  app.savefile().available()
}

#[command]
pub(crate) async fn begin<R: Runtime>(app: AppHandle<R>, name: String, mime: String) -> Result<Begun> {
  app.savefile().begin(&name, &mime)
}

#[command]
pub(crate) async fn write<R: Runtime>(app: AppHandle<R>, id: String, data: String) -> Result<()> {
  app.savefile().write(&id, &data)
}

#[command]
pub(crate) async fn finish<R: Runtime>(app: AppHandle<R>, id: String) -> Result<Saved> {
  app.savefile().finish(&id)
}

#[command]
pub(crate) async fn cancel<R: Runtime>(app: AppHandle<R>, id: String) -> Result<()> {
  app.savefile().cancel(&id)
}
