//! ASWebAuthenticationSession on a Mac, so the system's saved passkeys answer the page
//! (RFC 8252). Nothing here logs: the completion carries a one-use code.

use std::cell::RefCell;
use std::sync::Arc;

use block2::RcBlock;
use objc2::rc::Retained;
use objc2::runtime::ProtocolObject;
use objc2::{AnyThread, DefinedClass, MainThreadMarker, MainThreadOnly, define_class, msg_send};
use objc2_app_kit::NSWindow;
use objc2_authentication_services::{
  ASPresentationAnchor, ASWebAuthenticationPresentationContextProviding, ASWebAuthenticationSession,
};
use objc2_foundation::{NSError, NSObject, NSObjectProtocol, NSString, NSURL};
use serde_json::json;
use tauri::{AppHandle, Manager, Runtime, Url};

use crate::error::{Error, Result};
use crate::listeners::Listeners;
use crate::rules;

struct AnchorIvars {
  window: Retained<NSWindow>,
}

define_class!(
  #[unsafe(super(NSObject))]
  #[thread_kind = MainThreadOnly]
  #[name = "CoilycoSignInAnchor"]
  #[ivars = AnchorIvars]
  struct Anchor;

  unsafe impl NSObjectProtocol for Anchor {}

  unsafe impl ASWebAuthenticationPresentationContextProviding for Anchor {
    #[unsafe(method_id(presentationAnchorForWebAuthenticationSession:))]
    fn anchor(&self, _session: &ASWebAuthenticationSession) -> Retained<ASPresentationAnchor> {
      Retained::into_super(Retained::into_super(self.ivars().window.clone()))
    }
  }
);

impl Anchor {
  fn new(mtm: MainThreadMarker, window: Retained<NSWindow>) -> Retained<Self> {
    let this = Self::alloc(mtm).set_ivars(AnchorIvars { window });
    unsafe { msg_send![super(this), init] }
  }
}

/// The session holds its presentation provider weakly, so both live until it completes.
struct Active {
  _session: Retained<ASWebAuthenticationSession>,
  _anchor: Retained<Anchor>,
}

thread_local! {
  // Main thread only: begin starts a session there and its completion ends it there.
  static ACTIVE: RefCell<Option<Active>> = const { RefCell::new(None) };
}

fn other(message: &str) -> Error {
  Error { code: "other".into(), message: message.into() }
}

pub fn start<R: Runtime>(app: &AppHandle<R>, listeners: Arc<Listeners>, url: &Url) -> Result<()> {
  let (answer, wait) = std::sync::mpsc::channel();
  let (handle, url) = (app.clone(), url.clone());
  app.run_on_main_thread(move || {
    let _ = answer.send(begin(&handle, &listeners, &url));
  })?;
  wait.recv().map_err(|_| other("the main thread did not answer"))?
}

fn window_of<R: Runtime>(app: &AppHandle<R>) -> Result<Retained<NSWindow>> {
  let window = app.webview_windows().into_values().next().ok_or_else(|| other("no window to present from"))?;
  let raw = window.ns_window()?;
  // SAFETY: tauri hands back the window's own NSWindow, and retain adds a count.
  unsafe { Retained::retain(raw.cast::<NSWindow>()) }.ok_or_else(|| other("the window has no NSWindow"))
}

fn begin<R: Runtime>(app: &AppHandle<R>, listeners: &Arc<Listeners>, url: &Url) -> Result<()> {
  let mtm = MainThreadMarker::new().ok_or_else(|| other("not on the main thread"))?;
  let anchor = Anchor::new(mtm, window_of(app)?);
  let target = NSURL::URLWithString(&NSString::from_str(url.as_str())).ok_or_else(|| other("not an address"))?;
  let scheme = NSString::from_str(rules::SCHEME);
  let sink = listeners.clone();
  let completion = RcBlock::new(move |returned: *mut NSURL, _failure: *mut NSError| {
    ACTIVE.with(|active| active.borrow_mut().take());
    // SAFETY: AppKit passes nil or a live object for the length of this call.
    let address = unsafe { returned.as_ref() }.and_then(|returned| returned.absoluteString()).map(|text| text.to_string());
    match address.filter(|address| rules::is_return(address)) {
      Some(url) => sink.emit("return", &json!({ "url": url })),
      // A cancel and every other failure alike: the page needs to know it is gone.
      None => sink.emit("closed", &json!({})),
    };
  });
  #[allow(deprecated)] // the callback-object initialiser needs macOS 14.5, and the app's floor is 13
  let session = unsafe {
    ASWebAuthenticationSession::initWithURL_callbackURLScheme_completionHandler(
      ASWebAuthenticationSession::alloc(),
      &target,
      Some(&scheme),
      RcBlock::as_ptr(&completion).cast(),
    )
  };
  unsafe { session.setPresentationContextProvider(Some(ProtocolObject::from_ref(&*anchor))) };
  ACTIVE.with(|active| *active.borrow_mut() = Some(Active { _session: session.clone(), _anchor: anchor }));
  // SAFETY: the session is configured and on the main thread, which start requires.
  if unsafe { session.start() } {
    Ok(())
  } else {
    ACTIVE.with(|active| active.borrow_mut().take());
    Err(other("the session did not start"))
  }
}
