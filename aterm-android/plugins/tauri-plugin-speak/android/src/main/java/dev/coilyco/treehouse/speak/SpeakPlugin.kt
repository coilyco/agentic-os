package dev.coilyco.treehouse.speak

import android.app.Activity
import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import android.util.Log
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import java.util.Locale
import org.json.JSONObject

@InvokeArg
class SpeakArgs {
    var id: String = ""
    var text: String = ""
    var language: String? = null
    var rate: Float? = null
}

private const val TAG = "TreehouseSpeak"
private const val INIT_WAIT_MS = 4000L

// One TextToSpeech kept for the app's life, one reply at a time, read by `poll`.
// Design: references/android-speak.md.
@TauriPlugin
class SpeakPlugin(private val activity: Activity) : Plugin(activity) {
    private val main = Handler(Looper.getMainLooper())
    private val lock = Any()
    private var tts: TextToSpeech? = null
    private var starting = true
    private val waiting = mutableListOf<() -> Unit>()

    private var phase = "idle"
    private var id: String? = null
    private var begun = 0L
    private var startedMs: Long? = null
    private var doneMs: Long? = null
    private var error: Int? = null
    private var seq = 0
    private var pieces = 0
    private var focus: AudioFocusRequest? = null

    private val audio get() = activity.getSystemService(Context.AUDIO_SERVICE) as AudioManager

    override fun load(webView: android.webkit.WebView) {
        super.load(webView)
        main.post {
            tts = TextToSpeech(activity.applicationContext) { status ->
                main.post { engineReady(status == TextToSpeech.SUCCESS) }
            }
        }
    }

    private fun engineReady(ok: Boolean) {
        starting = false
        if (!ok) {
            Log.w(TAG, "engine failed to start")
            tts?.shutdown()
            tts = null
        } else {
            tts?.setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_ASSISTANT)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                    .build()
            )
            tts?.setOnUtteranceProgressListener(progress())
            Log.i(TAG, "engine ready ${tts?.defaultEngine} voices ${voices()}")
        }
        val pending = waiting.toList()
        waiting.clear()
        pending.forEach { it() }
    }

    private fun voices() = try { tts?.voices?.size ?: 0 } catch (e: Exception) { 0 }

    // Runs on the main thread once the engine has started or failed, or after a wait.
    private fun whenStarted(run: () -> Unit) {
        main.post {
            if (!starting) return@post run()
            var done = false
            val once = { if (!done) { done = true; run() } }
            waiting.add(once)
            main.postDelayed({ starting = false; once() }, INIT_WAIT_MS)
        }
    }

    private fun change(next: String, code: Int? = null) {
        synchronized(lock) {
            phase = next
            error = code
            seq += 1
            Log.i(TAG, "id=$id phase=$next elapsedMs=${SystemClock.elapsedRealtime() - begun}" + (code?.let { " error=$it" } ?: ""))
        }
    }

    private fun spoken() = JSObject().apply {
        synchronized(lock) {
            put("seq", seq)
            put("phase", phase)
            put("id", id ?: JSONObject.NULL)
            put("startedMs", startedMs ?: JSONObject.NULL)
            put("doneMs", doneMs ?: JSONObject.NULL)
            put("error", error ?: JSONObject.NULL)
        }
    }

    @Command
    fun available(invoke: Invoke) = whenStarted {
        invoke.resolve(JSObject().apply {
            put("available", tts != null)
            put("engine", tts?.defaultEngine ?: JSONObject.NULL)
            put("voices", voices())
        })
    }

    @Command
    fun speak(invoke: Invoke) {
        val args = invoke.parseArgs(SpeakArgs::class.java)
        if (args.text.isBlank()) return invoke.reject("There is nothing to read.", "empty")
        whenStarted {
            val engine = tts
            if (engine == null) return@whenStarted invoke.reject("This phone has no speech engine.", "unavailable")
            if (!takeFocus()) return@whenStarted invoke.reject("Another app holds the audio.", "focus")
            if (!args.language.isNullOrBlank()) {
                val set = engine.setLanguage(Locale.forLanguageTag(args.language!!))
                if (set < TextToSpeech.LANG_AVAILABLE) Log.w(TAG, "language ${args.language} not available ($set), keeping the engine's")
            }
            engine.setSpeechRate((args.rate ?: 1.0f).coerceIn(0.5f, 2.0f))
            val parts = cut(args.text, minOf(TextToSpeech.getMaxSpeechInputLength() - 100, 3900))
            synchronized(lock) {
                id = args.id
                begun = SystemClock.elapsedRealtime()
                startedMs = null
                doneMs = null
                pieces = parts.size
            }
            change("speaking")
            parts.forEachIndexed { index, part ->
                val mode = if (index == 0) TextToSpeech.QUEUE_FLUSH else TextToSpeech.QUEUE_ADD
                val queued = engine.speak(part, mode, null, "${args.id}#$index")
                if (queued != TextToSpeech.SUCCESS) {
                    change("error", queued)
                    leaveFocus()
                    return@whenStarted invoke.reject("The engine refused the text ($queued).", "other")
                }
            }
            invoke.resolve(JSObject().apply { put("id", args.id) })
        }
    }

    @Command
    fun stop(invoke: Invoke) {
        main.post {
            tts?.stop()
            leaveFocus()
            synchronized(lock) { if (phase == "speaking") change("idle") }
            invoke.resolve()
        }
    }

    @Command
    fun poll(invoke: Invoke) = invoke.resolve(spoken())

    private fun progress() = object : UtteranceProgressListener() {
        // A flushed reply still reports, and only the current one may change the state.
        private fun mine(utterance: String?) = utterance != null && synchronized(lock) { utterance.substringBeforeLast('#') == id }

        override fun onStart(utteranceId: String?) {
            if (!mine(utteranceId) || !utteranceId!!.endsWith("#0")) return
            synchronized(lock) { startedMs = SystemClock.elapsedRealtime() - begun }
            change("speaking")
        }

        override fun onDone(utteranceId: String?) {
            if (!mine(utteranceId) || !utteranceId!!.endsWith("#${pieces - 1}")) return
            synchronized(lock) { doneMs = SystemClock.elapsedRealtime() - begun }
            main.post { leaveFocus() }
            change("done")
        }

        @Deprecated("Replaced by onError(String, Int)")
        override fun onError(utteranceId: String?) = onError(utteranceId, TextToSpeech.ERROR)

        override fun onError(utteranceId: String?, errorCode: Int) {
            if (!mine(utteranceId)) return
            main.post { leaveFocus() }
            change("error", errorCode)
        }
    }

    // A call or another app's speech takes the audio, and reading stops with it.
    private fun takeFocus(): Boolean {
        leaveFocus()
        val request = AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK)
            .setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_ASSISTANT)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                    .build()
            )
            .setOnAudioFocusChangeListener { signal ->
                if (signal == AudioManager.AUDIOFOCUS_LOSS || signal == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT) {
                    main.post {
                        tts?.stop()
                        synchronized(lock) { if (phase == "speaking") change("idle") }
                        Log.i(TAG, "id=$id focus lost ($signal)")
                    }
                }
            }
            .build()
        val granted = audio.requestAudioFocus(request) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED
        if (granted) focus = request
        return granted
    }

    private fun leaveFocus() {
        focus?.let { audio.abandonAudioFocusRequest(it) }
        focus = null
    }

    // Cuts at sentence ends, so a long reply fits the engine's limit.
    private fun cut(text: String, limit: Int): List<String> {
        val whole = text.trim()
        if (whole.length <= limit) return listOf(whole)
        val parts = mutableListOf<String>()
        var rest = whole
        while (rest.length > limit) {
            val window = rest.substring(0, limit)
            val at = maxOf(window.lastIndexOf(". "), window.lastIndexOf("? "), window.lastIndexOf("! "), window.lastIndexOf('\n'))
            val end = if (at > limit / 2) at + 1 else window.lastIndexOf(' ').takeIf { it > 0 } ?: limit
            parts.add(rest.substring(0, end).trim())
            rest = rest.substring(end).trim()
        }
        if (rest.isNotEmpty()) parts.add(rest)
        return parts
    }
}
