use serde::Serialize;
use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::{Available, Begun, Saved};

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.savefile";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<SaveFile<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "SaveFilePlugin")?;
  Ok(SaveFile(handle))
}

#[derive(Serialize)]
struct BeginArgs<'a> {
  name: &'a str,
  mime: &'a str,
}

#[derive(Serialize)]
struct WriteArgs<'a> {
  id: &'a str,
  data: &'a str,
}

#[derive(Serialize)]
struct IdArgs<'a> {
  id: &'a str,
}

/// The Kotlin `SaveFilePlugin`, called through Tauri's mobile plugin bridge.
pub struct SaveFile<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> SaveFile<R> {
  pub fn available(&self) -> Result<Available> {
    Ok(self.0.run_mobile_plugin("available", ())?)
  }

  pub fn begin(&self, name: &str, mime: &str) -> Result<Begun> {
    Ok(self.0.run_mobile_plugin("begin", BeginArgs { name, mime })?)
  }

  pub fn write(&self, id: &str, data: &str) -> Result<()> {
    Ok(self.0.run_mobile_plugin("write", WriteArgs { id, data })?)
  }

  pub fn finish(&self, id: &str) -> Result<Saved> {
    Ok(self.0.run_mobile_plugin("finish", IdArgs { id })?)
  }

  pub fn cancel(&self, id: &str) -> Result<()> {
    Ok(self.0.run_mobile_plugin("cancel", IdArgs { id })?)
  }
}
