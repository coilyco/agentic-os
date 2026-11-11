use serde::Serialize;
use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::{Accepted, Availability, Spoken};

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.speak";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<Speak<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "SpeakPlugin")?;
  Ok(Speak(handle))
}

#[derive(Serialize)]
struct SpeakArgs {
  id: String,
  text: String,
  language: Option<String>,
  rate: Option<f32>,
}

/// The Kotlin `SpeakPlugin`, called through Tauri's mobile plugin bridge.
pub struct Speak<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> Speak<R> {
  pub fn available(&self) -> Result<Availability> {
    Ok(self.0.run_mobile_plugin("available", ())?)
  }

  pub fn speak(&self, id: String, text: String, language: Option<String>, rate: Option<f32>) -> Result<Accepted> {
    Ok(self.0.run_mobile_plugin("speak", SpeakArgs { id, text, language, rate })?)
  }

  pub fn stop(&self) -> Result<()> {
    Ok(self.0.run_mobile_plugin("stop", ())?)
  }

  pub fn poll(&self) -> Result<Spoken> {
    Ok(self.0.run_mobile_plugin("poll", ())?)
  }
}
