package dev.coilyco.treehouse.push

import android.app.Activity
import android.content.Intent
import android.os.Build
import android.webkit.WebView
import app.tauri.annotation.TauriPlugin
import app.tauri.annotation.Command
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import org.json.JSONObject

// The token and a tapped push, for the page. Nothing here logs the token or a message.
// Contract: tooling-aterm-client/references/android-push.md.
@TauriPlugin
class PushPlugin(private val activity: Activity) : Plugin(activity) {
    private val lock = Any()
    private var pending: JSObject? = null

    override fun load(webView: WebView) {
        live = this
        accept(activity.intent)
    }

    override fun onNewIntent(intent: Intent) {
        accept(intent)
    }

    override fun onResume() {
        inFront = true
    }

    override fun onPause() {
        inFront = false
    }

    override fun onDestroy() {
        inFront = false
        if (live === this) live = null
    }

    fun tokenChanged(token: String) {
        trigger("tokenRefresh", JSObject().apply { put("token", token) })
    }

    private fun accept(intent: Intent?) {
        if (intent?.getBooleanExtra(PushNotifier.TAP, false) != true) return
        val tap = JSObject().apply {
            put("kind", intent.getStringExtra(PushNotifier.KIND).orEmpty())
            put("daemon_id", intent.getStringExtra(PushNotifier.DAEMON_ID).orEmpty())
            put("session", intent.getStringExtra(PushNotifier.SESSION) ?: JSONObject.NULL)
            put("tag", intent.getStringExtra(PushNotifier.TAG).orEmpty())
        }
        // The same tap must not arrive again after the activity is recreated.
        for (key in listOf(PushNotifier.TAP, PushNotifier.KIND, PushNotifier.DAEMON_ID, PushNotifier.SESSION, PushNotifier.TAG)) {
            intent.removeExtra(key)
        }
        synchronized(lock) { pending = tap }
        trigger("tap", tap)
    }

    /** The FCM token, made on first call. `push_off` without Firebase config. */
    @Command
    fun register(invoke: Invoke) {
        if (FirebaseApp.getApps(activity).isEmpty()) {
            invoke.reject("this build has no push service", "push_off")
            return
        }
        val messaging = FirebaseMessaging.getInstance()
        messaging.isAutoInitEnabled = true
        messaging.token.addOnCompleteListener { task ->
            val token = if (task.isSuccessful) task.result else null
            if (token.isNullOrEmpty()) {
                // Never the exception's own text, which can carry project details.
                invoke.reject("the push service gave no token", "no_token")
                return@addOnCompleteListener
            }
            PushStore.save(activity, token)
            invoke.resolve(JSObject().apply {
                put("token", token)
                put("device", device())
            })
        }
    }

    @Command
    fun token(invoke: Invoke) {
        invoke.resolve(JSObject().apply { put("token", PushStore.load(activity) ?: JSONObject.NULL) })
    }

    /** The tap that started or woke the app, held until taken once. */
    @Command
    fun takeTap(invoke: Invoke) {
        val tap = synchronized(lock) { pending.also { pending = null } }
        invoke.resolve(JSObject().apply { put("tap", tap ?: JSONObject.NULL) })
    }

    private fun device(): String {
        val model = Build.MODEL?.trim().orEmpty()
        return (if (model.isEmpty()) "Android" else model).take(64)
    }

    companion object {
        @Volatile
        var live: PushPlugin? = null

        @Volatile
        var inFront = false
    }
}
