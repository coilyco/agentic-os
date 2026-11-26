use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::{Registered, Tap, Taken, Token};

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.push";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<Push<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "PushPlugin")?;
  Ok(Push(handle))
}

/// The Kotlin `PushPlugin`, called through Tauri's mobile plugin bridge.
pub struct Push<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> Push<R> {
  pub fn register(&self) -> Result<Registered> {
    Ok(self.0.run_mobile_plugin("register", ())?)
  }

  pub fn token(&self) -> Result<Option<String>> {
    let token: Token = self.0.run_mobile_plugin("token", ())?;
    Ok(token.token)
  }

  pub fn take_tap(&self) -> Result<Option<Tap>> {
    let taken: Taken = self.0.run_mobile_plugin("takeTap", ())?;
    Ok(taken.tap)
  }
}
