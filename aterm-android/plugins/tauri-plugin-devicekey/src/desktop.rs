use std::marker::PhantomData;

use tauri::{AppHandle, Runtime, plugin::PluginApi};

use crate::error::{Code, Error, Result};
use crate::models::{Biometric, Enrolled, Signed, Status};

pub fn init<R: Runtime, C: serde::de::DeserializeOwned>(
  _app: &AppHandle<R>,
  _api: PluginApi<R, C>,
) -> Result<DeviceKey<R>> {
  Ok(DeviceKey(PhantomData))
}

/// A desktop build has no Keystore. It exists so the crate tests on a laptop.
/// `fn() -> R` keeps it Send and Sync, which `manage` demands.
pub struct DeviceKey<R: Runtime>(PhantomData<fn() -> R>);

fn unsupported() -> Error {
  Error::new(Code::Unsupported, "a device key exists only in the Android app")
}

impl<R: Runtime> DeviceKey<R> {
  pub fn status(&self) -> Result<Status> {
    Ok(Status { enrolled: false, key_id: None, biometric: Biometric::Unsupported })
  }

  pub fn enroll(&self, _challenge: String) -> Result<Enrolled> {
    Err(unsupported())
  }

  pub fn sign(&self, _challenge: String, _prompt: Option<String>) -> Result<Signed> {
    Err(unsupported())
  }

  pub fn forget(&self) -> Result<()> {
    Ok(())
  }
}
