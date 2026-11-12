use serde::Serialize;
use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::Pending;
use crate::rules;

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.systembrowser";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<SystemBrowser<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "SystemBrowserPlugin")?;
  Ok(SystemBrowser(handle))
}

#[derive(Serialize)]
struct OpenArgs {
  url: String,
}

/// The Kotlin `SystemBrowserPlugin`, called through Tauri's mobile plugin bridge.
pub struct SystemBrowser<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> SystemBrowser<R> {
  /// Kotlin trusts what it is handed, so the one rule for what opens is checked here.
  pub fn open(&self, url: &str) -> Result<()> {
    let url = rules::check_open(url)?;
    Ok(self.0.run_mobile_plugin("open", OpenArgs { url: url.into() })?)
  }

  pub fn take_return(&self) -> Result<Option<String>> {
    let pending: Pending = self.0.run_mobile_plugin("takeReturn", ())?;
    Ok(pending.url)
  }
}
