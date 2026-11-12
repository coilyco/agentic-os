package dev.coilyco.treehouse.systembrowser

import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.webkit.WebView
import androidx.browser.customtabs.CustomTabsIntent
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import org.json.JSONObject

@InvokeArg
class OpenArgs {
    lateinit var url: String
}

// Opens a Custom Tab and hands the sign-in return to the page. Nothing here logs an
// address, since it carries a one-use code. Contract: references/system-browser.md.
@TauriPlugin
class SystemBrowserPlugin(private val activity: Activity) : Plugin(activity) {
    private val main = Handler(Looper.getMainLooper())
    private val lock = Any()
    private var pending: String? = null
    // Set by open, cleared by a return or a close. `covered`: the browser is showing.
    private var browsing = false
    private var covered = false
    private var closing: Runnable? = null

    override fun load(webView: WebView) {
        accept(activity.intent)
    }

    override fun onNewIntent(intent: Intent) {
        accept(intent)
    }

    override fun onPause() {
        synchronized(lock) { if (browsing) covered = true }
    }

    // A return arrives in onNewIntent before this. A resume with none, after the browser
    // covered the app, is a dismissal, since Custom Tabs gives no callback for one.
    override fun onResume() {
        val check = Runnable {
            val dismissed = synchronized(lock) {
                (browsing && covered).also { if (it) { browsing = false; covered = false } }
            }
            if (dismissed) trigger("closed", JSObject())
        }
        synchronized(lock) {
            if (!browsing || !covered) return
            closing?.let { main.removeCallbacks(it) }
            closing = check
        }
        main.postDelayed(check, DISMISS_GRACE_MS)
    }

    private fun accept(intent: Intent?) {
        val address = intent?.dataString ?: return
        if (intent.action != Intent.ACTION_VIEW || !SignInRules.isReturn(address)) return
        // The same intent must not arrive again after the activity is recreated.
        activity.intent = Intent(Intent.ACTION_MAIN)
        synchronized(lock) {
            pending = address
            browsing = false
            covered = false
            closing?.let { main.removeCallbacks(it) }
        }
        trigger("return", JSObject().apply { put("url", address) })
    }

    // The Rust side checked the address against its one allow rule before it got here.
    @Command
    fun open(invoke: Invoke) {
        val args = invoke.parseArgs(OpenArgs::class.java)
        activity.runOnUiThread {
            synchronized(lock) { browsing = true; covered = false }
            try {
                CustomTabsIntent.Builder().build().launchUrl(activity, Uri.parse(args.url))
                invoke.resolve()
            } catch (e: ActivityNotFoundException) {
                synchronized(lock) { browsing = false }
                invoke.reject("no browser on this phone can open the sign-in page", "no_browser")
            } catch (e: Exception) {
                // Never the exception's own text: it can repeat the address.
                synchronized(lock) { browsing = false }
                invoke.reject("the sign-in page did not open", "other")
            }
        }
    }

    /** The return that arrived while no page listened, held until taken. */
    @Command
    fun takeReturn(invoke: Invoke) {
        val address = synchronized(lock) { pending.also { pending = null } }
        invoke.resolve(JSObject().apply { put("url", address ?: JSONObject.NULL) })
    }

    private companion object {
        const val DISMISS_GRACE_MS = 1000L
    }
}
