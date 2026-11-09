use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::Shared;

const ANDROID_PACKAGE: &str = "dev.coilyco.treehouse.sharetarget";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<ShareTarget<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "ShareTargetPlugin")?;
  Ok(ShareTarget(handle))
}

/// The Kotlin `ShareTargetPlugin`, called through Tauri's mobile plugin bridge.
pub struct ShareTarget<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> ShareTarget<R> {
  pub fn take(&self) -> Result<Shared> {
    Ok(self.0.run_mobile_plugin("take", ())?)
  }
}
