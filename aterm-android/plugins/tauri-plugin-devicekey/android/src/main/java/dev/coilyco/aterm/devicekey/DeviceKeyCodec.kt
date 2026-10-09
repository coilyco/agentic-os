package dev.coilyco.aterm.devicekey

import java.security.MessageDigest
import java.util.Base64

/** The closed set of rejection codes. The Rust side parses the same strings. */
object Codes {
    const val CANCELLED = "cancelled"
    const val LOCKOUT = "lockout"
    const val NONE_ENROLLED = "none_enrolled"
    const val UNSUPPORTED = "unsupported"
    const val INVALIDATED = "invalidated"
    const val OTHER = "other"
}

class DeviceKeyException(val code: String, message: String, cause: Throwable? = null) : Exception(message, cause)

/** The bytes the daemon verifies. The signed message is built here, never by the page. */
object DeviceKeyCodec {
    const val MIN_CHALLENGE = 16
    const val MAX_CHALLENGE = 64

    private val ASSERT_PREFIX = "aterm-device-assert-v1".toByteArray(Charsets.US_ASCII)

    fun encode(bytes: ByteArray): String = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)

    /** Rejects padding and the standard alphabet, so a challenge has one spelling. */
    fun decodeChallenge(text: String?): ByteArray {
        if (text.isNullOrEmpty() || !text.all { it.isLetterOrDigit() && it.code < 128 || it == '-' || it == '_' }) {
            throw DeviceKeyException(Codes.OTHER, "the challenge is not base64url without padding")
        }
        val bytes = try {
            Base64.getUrlDecoder().decode(text)
        } catch (e: IllegalArgumentException) {
            throw DeviceKeyException(Codes.OTHER, "the challenge is not base64url without padding", e)
        }
        if (bytes.size < MIN_CHALLENGE || bytes.size > MAX_CHALLENGE) {
            throw DeviceKeyException(Codes.OTHER, "the challenge must be $MIN_CHALLENGE to $MAX_CHALLENGE bytes")
        }
        return bytes
    }

    /** M = "aterm-device-assert-v1" || 0x00 || challenge. */
    fun assertionMessage(challenge: ByteArray): ByteArray = ASSERT_PREFIX + byteArrayOf(0) + challenge

    /** base64url of SHA-256 over the SubjectPublicKeyInfo DER. */
    fun keyId(spki: ByteArray): String = encode(MessageDigest.getInstance("SHA-256").digest(spki))
}
