use serde::Serialize;
use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::{Availability, Heard};

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.dictation";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<Dictation<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "DictationPlugin")?;
  Ok(Dictation(handle))
}

#[derive(Serialize)]
struct StartArgs {
  language: Option<String>,
}

/// The Kotlin `DictationPlugin`, called through Tauri's mobile plugin bridge.
pub struct Dictation<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> Dictation<R> {
  pub fn available(&self) -> Result<Availability> {
    Ok(self.0.run_mobile_plugin("available", ())?)
  }

  pub fn start(&self, language: Option<String>) -> Result<()> {
    Ok(self.0.run_mobile_plugin("start", StartArgs { language })?)
  }

  pub fn poll(&self) -> Result<Heard> {
    Ok(self.0.run_mobile_plugin("poll", ())?)
  }

  pub fn stop(&self) -> Result<()> {
    Ok(self.0.run_mobile_plugin("stop", ())?)
  }

  pub fn cancel(&self) -> Result<()> {
    Ok(self.0.run_mobile_plugin("cancel", ())?)
  }
}
