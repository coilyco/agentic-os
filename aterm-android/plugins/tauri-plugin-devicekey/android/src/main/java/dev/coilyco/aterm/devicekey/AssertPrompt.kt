package dev.coilyco.aterm.devicekey

import android.app.Activity
import android.hardware.biometrics.BiometricPrompt
import android.os.CancellationSignal
import java.security.Signature
import java.util.concurrent.atomic.AtomicBoolean

/** One prompt at a time. The CryptoObject completes the signature after it passes. */
class AssertPrompt(private val activity: Activity) {
    private val open = AtomicBoolean(false)

    fun sign(
        signature: Signature,
        message: ByteArray,
        title: String,
        onSigned: (ByteArray) -> Unit,
        onFailed: (DeviceKeyException) -> Unit,
    ) {
        if (!open.compareAndSet(false, true)) {
            onFailed(DeviceKeyException(Codes.OTHER, "a prompt is already open"))
            return
        }
        activity.runOnUiThread {
            try {
                val prompt = BiometricPrompt.Builder(activity)
                    .setTitle(title)
                    .setAllowedAuthenticators(DeviceKeyStore.AUTHENTICATORS)
                    .setConfirmationRequired(false)
                    .build()
                prompt.authenticate(
                    BiometricPrompt.CryptoObject(signature),
                    CancellationSignal(),
                    activity.mainExecutor,
                    callback(message, onSigned, onFailed),
                )
            } catch (e: Exception) {
                open.set(false)
                onFailed(DeviceKeyException(Codes.OTHER, "the prompt did not open: ${e.message}", e))
            }
        }
    }

    private fun callback(
        message: ByteArray,
        onSigned: (ByteArray) -> Unit,
        onFailed: (DeviceKeyException) -> Unit,
    ) = object : BiometricPrompt.AuthenticationCallback() {
        override fun onAuthenticationError(errorCode: Int, errString: CharSequence?) {
            open.set(false)
            onFailed(DeviceKeyException(BiometricErrors.codeFor(errorCode), errString?.toString() ?: "authentication failed"))
        }

        override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
            open.set(false)
            try {
                val signature = result.cryptoObject?.signature
                    ?: throw DeviceKeyException(Codes.OTHER, "the prompt returned no signature")
                signature.update(message)
                onSigned(signature.sign())
            } catch (e: DeviceKeyException) {
                onFailed(e)
            } catch (e: Exception) {
                onFailed(DeviceKeyException(Codes.OTHER, "signing failed: ${e.message}", e))
            }
        }
        // A wrong fingerprint keeps the prompt open, so onAuthenticationFailed is left.
    }
}
