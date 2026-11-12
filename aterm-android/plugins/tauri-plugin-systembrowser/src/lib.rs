//! Opens the system browser for sign-in and hands the dev.coilyco.treehouse:/signin
//! return to the page. Contract: tooling-aterm-client/references/system-browser.md.

use tauri::{
  Manager, Runtime,
  plugin::{Builder, TauriPlugin},
};

mod commands;
#[cfg(desktop)]
mod desktop;
mod error;
#[cfg(desktop)]
mod listeners;
#[cfg(mobile)]
mod mobile;
mod models;
pub mod rules;

#[cfg(desktop)]
use desktop::SystemBrowser;
#[cfg(mobile)]
use mobile::SystemBrowser;

pub use error::{Error, Result};
pub use models::{Pending, Returned};

/// Extensions to [`tauri::App`] and [`tauri::AppHandle`] for the system browser.
pub trait SystemBrowserExt<R: Runtime> {
  fn systembrowser(&self) -> &SystemBrowser<R>;
}

impl<R: Runtime, T: Manager<R>> SystemBrowserExt<R> for T {
  fn systembrowser(&self) -> &SystemBrowser<R> {
    self.state::<SystemBrowser<R>>().inner()
  }
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
  let builder = Builder::new("systembrowser").setup(|app, api| {
    #[cfg(mobile)]
    let systembrowser = mobile::init(app, api)?;
    #[cfg(desktop)]
    let systembrowser = desktop::init(app, api)?;
    app.manage(systembrowser);
    Ok(())
  });
  #[cfg(mobile)]
  let builder = builder.invoke_handler(tauri::generate_handler![commands::open, commands::take_return]);
  #[cfg(desktop)]
  let builder = builder
    .invoke_handler(tauri::generate_handler![
      commands::open,
      commands::take_return,
      commands::register_listener,
      commands::remove_listener
    ])
    .on_page_load(|webview, payload| {
      if payload.event() == tauri::webview::PageLoadEvent::Started {
        webview.state::<SystemBrowser<R>>().remove_webview_listeners(webview.label());
      }
    });
  builder.build()
}
