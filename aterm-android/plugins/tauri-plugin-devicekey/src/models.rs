use serde::{Deserialize, Serialize};

/// Whether the phone can prompt for a fingerprint or screen lock right now.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Biometric {
  Ready,
  NoneEnrolled,
  Unsupported,
  /// The key exists but Android no longer lets it sign, so enroll again.
  Invalidated,
}

/// `status`. Never prompts. `key_id` is null while `enrolled` is false.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Status {
  pub enrolled: bool,
  pub key_id: Option<String>,
  pub biometric: Biometric,
}

/// `enroll`. Every string is base64url without padding, the attestation chain leaf first.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Enrolled {
  pub key_id: String,
  pub public_key: String,
  pub attestation: Vec<String>,
}

/// `sign`. `signature` is base64url DER ECDSA over SHA-256 of the assertion message.
#[derive(Debug, Clone, PartialEq, Eq, Deserialize, Serialize)]
pub struct Signed {
  pub key_id: String,
  pub signature: String,
}

#[cfg(mobile)]
#[derive(Debug, Serialize)]
pub(crate) struct EnrollArgs {
  pub challenge: String,
}

#[cfg(mobile)]
#[derive(Debug, Serialize)]
pub(crate) struct SignArgs {
  pub challenge: String,
  pub prompt: Option<String>,
}

#[cfg(test)]
mod tests {
  use super::*;
  use serde_json::json;

  #[test]
  fn status_wire_shape() {
    let status = Status { enrolled: true, key_id: Some("abc".into()), biometric: Biometric::NoneEnrolled };
    assert_eq!(
      serde_json::to_value(&status).unwrap(),
      json!({"enrolled": true, "key_id": "abc", "biometric": "none_enrolled"})
    );
    let blank = Status { enrolled: false, key_id: None, biometric: Biometric::Ready };
    assert_eq!(
      serde_json::to_value(&blank).unwrap(),
      json!({"enrolled": false, "key_id": null, "biometric": "ready"})
    );
  }

  #[test]
  fn enrolled_and_signed_wire_shape() {
    let enrolled = Enrolled { key_id: "k".into(), public_key: "p".into(), attestation: vec!["leaf".into(), "root".into()] };
    assert_eq!(
      serde_json::to_value(&enrolled).unwrap(),
      json!({"key_id": "k", "public_key": "p", "attestation": ["leaf", "root"]})
    );
    let signed = Signed { key_id: "k".into(), signature: "s".into() };
    assert_eq!(serde_json::to_value(&signed).unwrap(), json!({"key_id": "k", "signature": "s"}));
  }

  #[test]
  fn kotlin_replies_parse_into_the_same_shapes() {
    let status: Status = serde_json::from_value(json!({"enrolled": true, "key_id": "k", "biometric": "invalidated"})).unwrap();
    assert_eq!(status.biometric, Biometric::Invalidated);
    assert!(serde_json::from_value::<Status>(json!({"enrolled": true, "key_id": "k", "biometric": "soon"})).is_err());
  }
}
