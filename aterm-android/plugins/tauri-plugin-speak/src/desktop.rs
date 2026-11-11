use std::marker::PhantomData;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::Result;
use crate::models::{Accepted, Availability, Phase, Spoken};

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<Speak<R>> {
  Ok(Speak(PhantomData))
}

/// No speech bridge on the Mac yet, so the page offers no read-aloud.
/// `fn() -> R` keeps it Send and Sync.
pub struct Speak<R: Runtime>(PhantomData<fn() -> R>);

impl<R: Runtime> Speak<R> {
  pub fn available(&self) -> Result<Availability> {
    Ok(Availability { available: false, engine: None, voices: 0 })
  }

  pub fn speak(&self, _id: String, _text: String, _language: Option<String>, _rate: Option<f32>) -> Result<Accepted> {
    Err(crate::Error { code: "unavailable".into(), message: "Speech is not built for this desktop yet.".into() })
  }

  pub fn stop(&self) -> Result<()> {
    Ok(())
  }

  pub fn poll(&self) -> Result<Spoken> {
    Ok(Spoken { seq: 0, phase: Phase::Idle, id: None, started_ms: None, done_ms: None, error: None })
  }
}
