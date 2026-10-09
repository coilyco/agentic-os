use tauri::{
  AppHandle, Runtime,
  plugin::{PluginApi, PluginHandle},
};

use crate::error::Result;
use crate::models::{EnrollArgs, Enrolled, SignArgs, Signed, Status};

const ANDROID_PACKAGE: &str = "dev.coilyco.aterm.devicekey";

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  api: PluginApi<R, C>,
) -> Result<DeviceKey<R>> {
  let handle = api.register_android_plugin(ANDROID_PACKAGE, "DeviceKeyPlugin")?;
  Ok(DeviceKey(handle))
}

/// The Kotlin `DeviceKeyPlugin`, called through Tauri's mobile plugin bridge.
pub struct DeviceKey<R: Runtime>(PluginHandle<R>);

impl<R: Runtime> DeviceKey<R> {
  pub fn status(&self) -> Result<Status> {
    Ok(self.0.run_mobile_plugin("status", ())?)
  }

  pub fn enroll(&self, challenge: String) -> Result<Enrolled> {
    Ok(self.0.run_mobile_plugin("enroll", EnrollArgs { challenge })?)
  }

  pub fn sign(&self, challenge: String, prompt: Option<String>) -> Result<Signed> {
    Ok(self.0.run_mobile_plugin("sign", SignArgs { challenge, prompt })?)
  }

  pub fn forget(&self) -> Result<()> {
    Ok(self.0.run_mobile_plugin("forget", ())?)
  }
}
