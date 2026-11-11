package dev.coilyco.treehouse.dictation

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.speech.RecognitionService
import android.speech.SpeechRecognizer
import android.util.Log
import java.io.File
import org.json.JSONObject
import org.vosk.Model
import org.vosk.Recognizer

// Debug builds only. A real offline recognizer, so CI hears the emulator's mic.
// The lane pushes the model, nothing bundles it. Design: references/android-dictation.md.
class VoskRecognitionService : RecognitionService() {
    private val main = Handler(Looper.getMainLooper())
    @Volatile private var session: Session? = null

    override fun onCreate() {
        super.onCreate()
        Thread { model(File(filesDir, MODEL_DIR)) }.start()
    }

    override fun onStartListening(recognizerIntent: Intent, listener: Callback) {
        session?.abort()
        session = Session(listener).also { Thread(it, "vosk-session").start() }
    }

    override fun onStopListening(listener: Callback) {
        session?.finish = true
    }

    override fun onCancel(listener: Callback) {
        session?.abort()
        session = null
    }

    private inner class Session(private val listener: Callback) : Runnable {
        @Volatile var finish = false
        @Volatile private var aborted = false

        fun abort() {
            aborted = true
        }

        private fun say(block: () -> Unit) {
            main.post { if (!aborted) block() }
        }

        private fun words(text: String) = Bundle().apply {
            putStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION, arrayListOf(text))
        }

        override fun run() {
            val loaded = model(File(filesDir, MODEL_DIR))
            if (loaded == null) return say { listener.error(SpeechRecognizer.ERROR_CLIENT) }
            if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
                return say { listener.error(SpeechRecognizer.ERROR_INSUFFICIENT_PERMISSIONS) }
            }
            val size = maxOf(AudioRecord.getMinBufferSize(RATE, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT), 8192)
            val mic = AudioRecord(MediaRecorder.AudioSource.MIC, RATE, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, size)
            val recognizer = Recognizer(loaded, RATE.toFloat())
            val chunk = ShortArray(2048)
            var last = ""
            var peak = 0
            var logged = System.currentTimeMillis()
            val started = System.currentTimeMillis()
            try {
                mic.startRecording()
                Log.i(TAG, "listening, mic state ${mic.recordingState}")
                say { listener.readyForSpeech(Bundle()) }
                say { listener.beginningOfSpeech() }
                while (!aborted && !finish) {
                    val read = mic.read(chunk, 0, chunk.size)
                    if (read <= 0) continue
                    for (i in 0 until read) peak = maxOf(peak, Math.abs(chunk[i].toInt()))
                    if (System.currentTimeMillis() - logged > 1000) {
                        Log.i(TAG, "mic peak $peak, silenced ${mic.activeRecordingConfiguration?.isClientSilenced}")
                        peak = 0
                        logged = System.currentTimeMillis()
                    }
                    if (recognizer.acceptWaveForm(chunk, read)) {
                        val text = JSONObject(recognizer.result).optString("text")
                        if (text.isNotEmpty()) {
                            Log.i(TAG, "final: $text")
                            return say { listener.endOfSpeech(); listener.results(words(text)) }
                        }
                    } else {
                        val partial = JSONObject(recognizer.partialResult).optString("partial")
                        if (partial.isNotEmpty() && partial != last) {
                            last = partial
                            Log.i(TAG, "partial: $partial")
                            say { listener.partialResults(words(partial)) }
                        }
                    }
                    if (System.currentTimeMillis() - started > TIMEOUT_MS) {
                        return say { listener.error(SpeechRecognizer.ERROR_SPEECH_TIMEOUT) }
                    }
                }
                if (aborted) return
                val text = JSONObject(recognizer.finalResult).optString("text")
                say {
                    listener.endOfSpeech()
                    if (text.isEmpty()) listener.error(SpeechRecognizer.ERROR_NO_MATCH) else listener.results(words(text))
                }
            } finally {
                mic.release()
                recognizer.close()
            }
        }
    }

    companion object {
        private const val TAG = "VoskDictation"
        private const val MODEL_DIR = "vosk-model"
        private const val RATE = 16000
        private const val TIMEOUT_MS = 90_000L
        @Volatile private var loaded: Model? = null

        @Synchronized
        private fun model(dir: File): Model? {
            if (loaded == null && File(dir, "am").exists()) loaded = Model(dir.absolutePath)
            if (loaded == null) Log.w(TAG, "no model at $dir")
            return loaded
        }
    }
}
