package dev.coilyco.aterm.devicekey

import android.content.Context
import android.hardware.biometrics.BiometricManager
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.Signature
import java.security.UnrecoverableKeyException
import java.security.spec.ECGenParameterSpec

data class KeyStatus(val enrolled: Boolean, val keyId: String?, val biometric: String)

data class EnrolledKey(val keyId: String, val publicKey: String, val attestation: List<String>)

/** The one P-256 key in the AndroidKeyStore. The private half never leaves it. */
class DeviceKeyStore(private val context: Context) {
    companion object {
        const val ALIAS = "dev.coilyco.aterm.devicekey"
        const val AUTHENTICATORS =
            BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL
        private const val PROVIDER = "AndroidKeyStore"
    }

    private fun keyStore(): KeyStore = KeyStore.getInstance(PROVIDER).apply { load(null) }

    /** Whether a prompt can run now. It never shows one. */
    fun readiness(): String =
        when (context.getSystemService(BiometricManager::class.java)?.canAuthenticate(AUTHENTICATORS)) {
            BiometricManager.BIOMETRIC_SUCCESS -> "ready"
            BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED -> "none_enrolled"
            else -> "unsupported"
        }

    /** Silent: initSign alone is enough to see an invalidated key. */
    fun status(): KeyStatus {
        val store = keyStore()
        var biometric = readiness()
        val certificate = if (store.containsAlias(ALIAS)) store.getCertificate(ALIAS) else null
        if (certificate == null) return KeyStatus(false, null, biometric)
        if (biometric == "ready" && !canInitSign(store)) biometric = "invalidated"
        return KeyStatus(true, DeviceKeyCodec.keyId(certificate.publicKey.encoded), biometric)
    }

    private fun canInitSign(store: KeyStore): Boolean = try {
        val key = store.getKey(ALIAS, null) as? PrivateKey
        if (key == null) false else { Signature.getInstance("SHA256withECDSA").initSign(key); true }
    } catch (e: KeyPermanentlyInvalidatedException) {
        false
    } catch (e: UnrecoverableKeyException) {
        false
    }

    /** Replaces any earlier key. The attestation challenge is the daemon challenge. */
    fun enroll(challenge: ByteArray): EnrolledKey {
        when (readiness()) {
            "none_enrolled" -> throw DeviceKeyException(Codes.NONE_ENROLLED, "set up a fingerprint or screen lock first")
            "unsupported" -> throw DeviceKeyException(Codes.UNSUPPORTED, "this phone cannot prompt for a fingerprint or screen lock")
        }
        val store = keyStore()
        if (store.containsAlias(ALIAS)) store.deleteEntry(ALIAS)
        try {
            generate(challenge, strongBox = true)
        } catch (e: StrongBoxUnavailableException) {
            generate(challenge, strongBox = false)
        }
        val chain = store.getCertificateChain(ALIAS)
            ?: throw DeviceKeyException(Codes.OTHER, "the Keystore returned no certificate chain")
        val spki = chain[0].publicKey.encoded
        return EnrolledKey(
            keyId = DeviceKeyCodec.keyId(spki),
            publicKey = DeviceKeyCodec.encode(spki),
            attestation = chain.map { DeviceKeyCodec.encode(it.encoded) },
        )
    }

    private fun generate(challenge: ByteArray, strongBox: Boolean) {
        val spec = KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_SIGN)
            .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
            .setDigests(KeyProperties.DIGEST_SHA256)
            .setUserAuthenticationRequired(true)
            // Timeout 0 is per use: no grace window.
            .setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL)
            .setInvalidatedByBiometricEnrollment(true)
            .setAttestationChallenge(challenge)
            .setIsStrongBoxBacked(strongBox)
            .build()
        KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, PROVIDER).apply { initialize(spec) }.generateKeyPair()
    }

    /** A Signature initialised for signing, for a CryptoObject to unlock. */
    fun signatureForPrompt(): Pair<Signature, String> {
        val store = keyStore()
        val certificate = (if (store.containsAlias(ALIAS)) store.getCertificate(ALIAS) else null)
            ?: throw DeviceKeyException(Codes.OTHER, "no device key is enrolled")
        val keyId = DeviceKeyCodec.keyId(certificate.publicKey.encoded)
        try {
            val key = store.getKey(ALIAS, null) as? PrivateKey
                ?: throw DeviceKeyException(Codes.OTHER, "no device key is enrolled")
            return Signature.getInstance("SHA256withECDSA").apply { initSign(key) } to keyId
        } catch (e: KeyPermanentlyInvalidatedException) {
            throw DeviceKeyException(Codes.INVALIDATED, "the key stopped working, enroll again", e)
        } catch (e: UnrecoverableKeyException) {
            throw DeviceKeyException(Codes.INVALIDATED, "the key stopped working, enroll again", e)
        }
    }

    fun forget() {
        val store = keyStore()
        if (store.containsAlias(ALIAS)) store.deleteEntry(ALIAS)
    }
}
