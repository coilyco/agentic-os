package dev.coilyco.aterm.devicekey

import android.app.Activity
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.JSArray
import app.tauri.plugin.JSObject
import app.tauri.plugin.Invoke
import app.tauri.plugin.Plugin
import org.json.JSONObject

@InvokeArg
class EnrollArgs {
    var challenge: String? = null
}

@InvokeArg
class SignArgs {
    var challenge: String? = null
    var prompt: String? = null
}

/** The page-facing commands. The Rust types in models.rs pin their shapes. */
@TauriPlugin
class DeviceKeyPlugin(activity: Activity) : Plugin(activity) {
    private val store = DeviceKeyStore(activity)
    private val prompt = AssertPrompt(activity)

    @Command
    fun status(invoke: Invoke) = guarded(invoke) {
        val status = store.status()
        invoke.resolve(JSObject().apply {
            put("enrolled", status.enrolled)
            put("key_id", status.keyId ?: JSONObject.NULL)
            put("biometric", status.biometric)
        })
    }

    @Command
    fun enroll(invoke: Invoke) = guarded(invoke) {
        val challenge = DeviceKeyCodec.decodeChallenge(invoke.parseArgs(EnrollArgs::class.java).challenge)
        val key = store.enroll(challenge)
        invoke.resolve(JSObject().apply {
            put("key_id", key.keyId)
            put("public_key", key.publicKey)
            put("attestation", JSArray(key.attestation))
        })
    }

    @Command
    fun sign(invoke: Invoke) = guarded(invoke) {
        val args = invoke.parseArgs(SignArgs::class.java)
        val message = DeviceKeyCodec.assertionMessage(DeviceKeyCodec.decodeChallenge(args.challenge))
        val (signature, keyId) = store.signatureForPrompt()
        val title = args.prompt?.trim()?.take(64)?.takeIf { it.isNotEmpty() } ?: "Unlock aterm typing"
        prompt.sign(
            signature, message, title,
            onSigned = { der ->
                invoke.resolve(JSObject().apply {
                    put("key_id", keyId)
                    put("signature", DeviceKeyCodec.encode(der))
                })
            },
            onFailed = { e -> invoke.reject(e.message, e.code, e) },
        )
    }

    @Command
    fun forget(invoke: Invoke) = guarded(invoke) {
        store.forget()
        invoke.resolve()
    }

    private fun guarded(invoke: Invoke, body: () -> Unit) {
        try {
            body()
        } catch (e: DeviceKeyException) {
            invoke.reject(e.message, e.code, e)
        } catch (e: Exception) {
            invoke.reject(e.message ?: "unexpected failure", Codes.OTHER, e)
        }
    }
}
