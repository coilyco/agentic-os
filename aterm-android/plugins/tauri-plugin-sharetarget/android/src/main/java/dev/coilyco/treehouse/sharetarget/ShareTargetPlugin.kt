package dev.coilyco.treehouse.sharetarget

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.provider.OpenableColumns
import android.util.Base64
import android.webkit.WebView
import app.tauri.annotation.Command
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSArray
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import org.json.JSONObject
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import java.util.concurrent.Future

/** What one Share intent held once read: the answer to the page's `take`. */
private class Read(val images: List<JSObject>, val skipped: List<JSObject>, val text: String?)

// Holds shared images until the page calls `take`. Files are read on arrival, since the
// sender's read grant ends with the task. Design: references/android-sharetarget.md.
@TauriPlugin
class ShareTargetPlugin(private val activity: Activity) : Plugin(activity) {
    private val readers = Executors.newSingleThreadExecutor()
    private val answers = Executors.newCachedThreadPool()
    private val waiting = ArrayList<Future<Read>>()

    override fun load(webView: WebView) {
        accept(activity.intent)
    }

    override fun onNewIntent(intent: Intent) {
        accept(intent)
    }

    private fun accept(intent: Intent?) {
        val action = intent?.action
        if (action != Intent.ACTION_SEND && action != Intent.ACTION_SEND_MULTIPLE) return
        val uris = streams(intent)
        val text = ShareRules.sharedText(intent.getCharSequenceExtra(Intent.EXTRA_TEXT))
        // The same intent must not arrive again after the activity is recreated.
        activity.intent = Intent(Intent.ACTION_MAIN)
        if (uris.isEmpty() && text == null) return
        val job = readers.submit(Callable { read(uris, text) })
        synchronized(waiting) { waiting.add(job) }
    }

    @Suppress("DEPRECATION")
    private fun streams(intent: Intent): List<Uri> =
        if (intent.action == Intent.ACTION_SEND_MULTIPLE) {
            intent.getParcelableArrayListExtra<Uri>(Intent.EXTRA_STREAM).orEmpty()
        } else {
            listOfNotNull(intent.getParcelableExtra<Uri>(Intent.EXTRA_STREAM))
        }

    private fun read(uris: List<Uri>, text: String?): Read {
        val images = ArrayList<JSObject>()
        val skipped = ArrayList<JSObject>()
        for ((index, uri) in uris.withIndex()) {
            val mime = activity.contentResolver.getType(uri)
            val (label, size) = describe(uri)
            val name = ShareRules.displayName(label, mime ?: "image/", index)
            val reason = ShareRules.skipReason(index, mime, size)
            if (reason != null) {
                skipped.add(skip(name, reason))
                continue
            }
            try {
                val bytes = activity.contentResolver.openInputStream(uri)?.use { it.readNBytes((ShareRules.MAX_BYTES + 1).toInt()) }
                when {
                    bytes == null -> skipped.add(skip(name, ShareRules.UNREADABLE))
                    bytes.size > ShareRules.MAX_BYTES -> skipped.add(skip(name, ShareRules.TOO_BIG))
                    else -> images.add(JSObject().apply {
                        put("name", name)
                        put("mime", mime)
                        put("data", Base64.encodeToString(bytes, Base64.NO_WRAP))
                    })
                }
            } catch (e: Exception) {
                // A revoked grant, a deleted file, or an app that went away mid-read.
                skipped.add(skip(name, ShareRules.UNREADABLE))
            }
        }
        return Read(images, skipped, text)
    }

    private fun describe(uri: Uri): Pair<String?, Long?> =
        try {
            activity.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { cursor ->
                if (!cursor.moveToFirst()) return Pair(null, null)
                val name = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME).takeIf { it >= 0 }?.let { cursor.getString(it) }
                val size = cursor.getColumnIndex(OpenableColumns.SIZE).takeIf { it >= 0 && !cursor.isNull(it) }?.let { cursor.getLong(it) }
                Pair(name, size)
            } ?: Pair(null, null)
        } catch (e: Exception) {
            Pair(null, null)
        }

    private fun skip(name: String, reason: String) = JSObject().apply {
        put("name", name)
        put("reason", reason)
    }

    /** Hands over everything that waits and forgets it, so one share is taken once. */
    @Command
    fun take(invoke: Invoke) {
        val jobs = synchronized(waiting) { waiting.toList().also { waiting.clear() } }
        // Waiting happens off the calling thread, so a slow file never holds the bridge.
        answers.execute {
            try {
                val images = JSArray()
                val skipped = JSArray()
                val texts = ArrayList<String>()
                for (job in jobs) {
                    val read = job.get()
                    read.images.forEach { images.put(it) }
                    read.skipped.forEach { skipped.put(it) }
                    read.text?.let { texts.add(it) }
                }
                invoke.resolve(JSObject().apply {
                    put("images", images)
                    put("skipped", skipped)
                    put("text", if (texts.isEmpty()) JSONObject.NULL else texts.joinToString("\n"))
                })
            } catch (e: Exception) {
                invoke.reject(e.message ?: "unexpected failure", "other", e)
            }
        }
    }
}
