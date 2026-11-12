use tauri::Url;

use crate::error::{Error, Result};

/// The private-use scheme of both apps (RFC 8252).
pub const SCHEME: &str = "dev.coilyco.treehouse";
/// The one host the app opens. Exact, so `evil-accounts.coilyco.ai` fails.
pub const ACCOUNTS_HOST: &str = "accounts.coilyco.ai";
const SIGN_IN_PATH: &str = "/signin";

/// Parses the page's address, or says in a fixed phrase why it will not be opened.
pub fn check_open(raw: &str) -> Result<Url> {
  let url = Url::parse(raw).map_err(|_| Error::refused("not an address"))?;
  if url.scheme() != "https" {
    return Err(Error::refused("not https"));
  }
  if url.host_str() != Some(ACCOUNTS_HOST) {
    return Err(Error::refused("not the accounts host"));
  }
  if !url.username().is_empty() || url.password().is_some() || url.port().is_some() {
    return Err(Error::refused("carries credentials or a port"));
  }
  if url.path() != SIGN_IN_PATH {
    return Err(Error::refused("not the sign-in page"));
  }
  Ok(url)
}

/// Whether an address is the app's own return, `dev.coilyco.treehouse:/signin?...`.
pub fn is_return(raw: &str) -> bool {
  Url::parse(raw).is_ok_and(|url| url.scheme() == SCHEME && url.host().is_none() && url.path() == SIGN_IN_PATH)
}

#[cfg(test)]
mod tests {
  use super::*;

  #[test]
  fn the_accounts_sign_in_page_opens_with_its_query() {
    let url = check_open("https://accounts.coilyco.ai/signin?state=s&code_challenge=c&redirect_uri=dev.coilyco.treehouse%3A%2Fsignin").unwrap();
    assert_eq!(url.host_str(), Some(ACCOUNTS_HOST));
  }

  #[test]
  fn every_other_address_is_refused_with_its_own_reason() {
    let cases = [
      ("not a url", "not an address"),
      ("http://accounts.coilyco.ai/signin", "not https"),
      ("dev.coilyco.treehouse:/signin", "not https"),
      ("https://accounts.coilyco.ai.evil.example/signin", "not the accounts host"),
      ("https://evil-accounts.coilyco.ai/signin", "not the accounts host"),
      ("https://accounts.coilyco.dev/signin", "not the accounts host"),
      ("https://accounts.coilyco.ai@evil.example/signin", "not the accounts host"),
      ("https://user@accounts.coilyco.ai/signin", "carries credentials or a port"),
      ("https://accounts.coilyco.ai:8443/signin", "carries credentials or a port"),
      ("https://accounts.coilyco.ai/signin/extra", "not the sign-in page"),
      ("https://accounts.coilyco.ai/", "not the sign-in page"),
    ];
    for (raw, reason) in cases {
      let error = check_open(raw).expect_err(raw);
      assert_eq!((error.code.as_str(), error.message.as_str()), ("refused", reason), "{raw}");
    }
  }

  #[test]
  fn a_refusal_never_repeats_the_address() {
    let error = check_open("https://accounts.coilyco.ai/signin?code=secret&x=").and_then(|_| check_open("http://x/?code=secret")).unwrap_err();
    assert!(!error.message.contains("secret"));
  }

  #[test]
  fn only_the_apps_own_signin_return_counts() {
    assert!(is_return("dev.coilyco.treehouse:/signin?code=c&state=s"));
    assert!(!is_return("dev.coilyco.treehouse:/other?code=c"));
    assert!(!is_return("dev.coilyco.treehouse://signin?code=c"));
    assert!(!is_return("dev.coilyco.treehouse.desktop:/signin?code=c"));
    assert!(!is_return("https://accounts.coilyco.ai/signin"));
    assert!(!is_return("tauri://localhost/signin"));
  }
}
