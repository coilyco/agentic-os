use std::marker::PhantomData;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::Result;
use crate::models::{Availability, Heard, Phase};

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<Dictation<R>> {
  Ok(Dictation(PhantomData))
}

/// No recognizer bridge on the Mac yet, so the page shows no mic.
/// `fn() -> R` keeps it Send and Sync.
pub struct Dictation<R: Runtime>(PhantomData<fn() -> R>);

impl<R: Runtime> Dictation<R> {
  pub fn available(&self) -> Result<Availability> {
    Ok(Availability { available: false, permission: "denied".into() })
  }

  pub fn start(&self, _language: Option<String>) -> Result<()> {
    Err(crate::Error { code: "unavailable".into(), message: "Dictation is not built for this desktop yet.".into() })
  }

  pub fn poll(&self) -> Result<Heard> {
    Ok(Heard { phase: Phase::Idle, text: String::new(), error: None, seq: 0 })
  }

  pub fn stop(&self) -> Result<()> {
    Ok(())
  }

  pub fn cancel(&self) -> Result<()> {
    Ok(())
  }
}
