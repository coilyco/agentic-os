package dev.coilyco.treehouse.dictation

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.speech.RecognitionListener
import android.speech.RecognizerIntent
import android.speech.SpeechRecognizer
import app.tauri.PermissionState
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.Permission
import app.tauri.annotation.PermissionCallback
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import org.json.JSONObject

@InvokeArg
class StartArgs {
    var language: String? = null
}

private const val MIC = "microphone"

// One recognizer at a time, read by the page's `poll`. Main thread only.
// Design: references/android-dictation.md.
@TauriPlugin(permissions = [Permission(strings = [Manifest.permission.RECORD_AUDIO], alias = MIC)])
class DictationPlugin(private val activity: Activity) : Plugin(activity) {
    private var recognizer: SpeechRecognizer? = null
    private val lock = Any()
    private var phase = "idle"
    private var text = ""
    private var error: Int? = null
    private var seq = 0

    private fun change(next: String, words: String = text, code: Int? = null) = synchronized(lock) {
        phase = next
        text = words
        error = code
        seq += 1
    }

    private fun heard() = JSObject().apply {
        synchronized(lock) {
            put("phase", phase)
            put("text", text)
            put("error", error ?: JSONObject.NULL)
            put("seq", seq)
        }
    }

    private fun granted() = getPermissionState(MIC) == PermissionState.GRANTED

    @Command
    fun available(invoke: Invoke) {
        invoke.resolve(JSObject().apply {
            put("available", SpeechRecognizer.isRecognitionAvailable(activity))
            put("permission", when (getPermissionState(MIC)) {
                PermissionState.GRANTED -> "granted"
                PermissionState.DENIED -> "denied"
                else -> "prompt"
            })
        })
    }

    @Command
    fun start(invoke: Invoke) {
        if (!SpeechRecognizer.isRecognitionAvailable(activity)) {
            invoke.reject("This phone has no speech recognition service.", "unavailable")
            return
        }
        if (granted()) begin(invoke) else requestPermissionForAlias(MIC, invoke, "micAnswered")
    }

    @PermissionCallback
    fun micAnswered(invoke: Invoke) {
        if (granted()) begin(invoke) else invoke.reject("The microphone is not allowed for this app.", "denied")
    }

    private fun begin(invoke: Invoke) {
        val language = invoke.parseArgs(StartArgs::class.java).language
        activity.runOnUiThread {
            try {
                recognizer?.destroy()
                change("listening", "")
                val made = SpeechRecognizer.createSpeechRecognizer(activity)
                made.setRecognitionListener(listener())
                made.startListening(Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH).apply {
                    putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM)
                    putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true)
                    putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 1)
                    putExtra(RecognizerIntent.EXTRA_CALLING_PACKAGE, activity.packageName)
                    if (!language.isNullOrBlank()) putExtra(RecognizerIntent.EXTRA_LANGUAGE, language)
                })
                recognizer = made
                invoke.resolve()
            } catch (e: Exception) {
                change("idle", "")
                invoke.reject(e.message ?: "The recognizer would not start.", "other", e)
            }
        }
    }

    private fun listener() = object : RecognitionListener {
        private fun best(results: Bundle?) =
            results?.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION)?.firstOrNull().orEmpty()

        override fun onPartialResults(partialResults: Bundle?) {
            val words = best(partialResults)
            if (words.isNotEmpty()) change("listening", words)
        }

        override fun onResults(results: Bundle?) = change("done", best(results))

        // Words heard before an error stay.
        override fun onError(code: Int) = change("failed", code = code)

        override fun onReadyForSpeech(params: Bundle?) {}
        override fun onBeginningOfSpeech() {}
        override fun onRmsChanged(rmsdB: Float) {}
        override fun onBufferReceived(buffer: ByteArray?) {}
        override fun onEndOfSpeech() {}
        override fun onEvent(eventType: Int, params: Bundle?) {}
    }

    @Command
    fun poll(invoke: Invoke) = invoke.resolve(heard())

    /** Asks for the final words, which arrive through `onResults`. */
    @Command
    fun stop(invoke: Invoke) {
        activity.runOnUiThread {
            recognizer?.stopListening()
            invoke.resolve()
        }
    }

    @Command
    fun cancel(invoke: Invoke) {
        activity.runOnUiThread {
            recognizer?.cancel()
            recognizer?.destroy()
            recognizer = null
            change("idle", "")
            invoke.resolve()
        }
    }
}
