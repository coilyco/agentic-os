package dev.coilyco.aterm.devicekey

import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class DeviceKeyCodecTest {
    private val challenge = ByteArray(32) { it.toByte() }

    private fun rejects(text: String?) {
        try {
            DeviceKeyCodec.decodeChallenge(text)
            fail("expected a rejection for $text")
        } catch (e: DeviceKeyException) {
            assertEquals(Codes.OTHER, e.code)
        }
    }

    @Test
    fun acceptsAThirtyTwoByteBase64UrlChallenge() {
        assertArrayEquals(challenge, DeviceKeyCodec.decodeChallenge(DeviceKeyCodec.encode(challenge)))
    }

    @Test
    fun rejectsPaddingStandardAlphabetAndBadLengths() {
        rejects(null)
        rejects("")
        rejects(Base64.getUrlEncoder().encodeToString(challenge)) // padded
        rejects("a+b/".repeat(11)) // standard alphabet
        rejects(DeviceKeyCodec.encode(ByteArray(15)))
        rejects(DeviceKeyCodec.encode(ByteArray(65)))
    }

    @Test
    fun messageIsPrefixThenZeroThenChallenge() {
        val message = DeviceKeyCodec.assertionMessage(challenge)
        val prefix = "aterm-device-assert-v1".toByteArray(Charsets.US_ASCII)
        assertArrayEquals(prefix, message.copyOfRange(0, prefix.size))
        assertEquals(0, message[prefix.size].toInt())
        assertArrayEquals(challenge, message.copyOfRange(prefix.size + 1, message.size))
    }

    @Test
    fun keyIdIsFortyThreeUrlSafeCharacters() {
        val id = DeviceKeyCodec.keyId(byteArrayOf(1, 2, 3))
        assertEquals(43, id.length)
        assertTrue(id.all { it.isLetterOrDigit() || it == '-' || it == '_' })
    }

    @Test
    fun aDerSignatureOverTheMessageVerifiesAndOneOverTheBareChallengeDoesNot() {
        val pair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
        val message = DeviceKeyCodec.assertionMessage(challenge)
        val der = Signature.getInstance("SHA256withECDSA").run { initSign(pair.private); update(message); sign() }
        val verify = { signed: ByteArray ->
            Signature.getInstance("SHA256withECDSA").run { initVerify(pair.public); update(signed); verify(der) }
        }
        assertTrue(verify(message))
        assertFalse(verify(challenge))
    }

    @Test
    fun biometricErrorsMapToTheClosedCodeSet() {
        val cases = mapOf(
            10 to Codes.CANCELLED, 5 to Codes.CANCELLED,
            7 to Codes.LOCKOUT, 9 to Codes.LOCKOUT,
            11 to Codes.NONE_ENROLLED, 14 to Codes.NONE_ENROLLED,
            12 to Codes.UNSUPPORTED, 1 to Codes.UNSUPPORTED, 15 to Codes.UNSUPPORTED,
            3 to Codes.OTHER, 8 to Codes.OTHER, 999 to Codes.OTHER,
        )
        for ((error, code) in cases) assertEquals("error $error", code, BiometricErrors.codeFor(error))
    }
}
