package dev.coilyco.treehouse.savefile

import android.app.Activity
import android.content.ContentValues
import android.net.Uri
import android.os.Environment
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import java.io.OutputStream

@InvokeArg
class BeginArgs {
    var name: String? = null
    var mime: String? = null
}

@InvokeArg
class WriteArgs {
    var id: String? = null
    var data: String? = null
}

@InvokeArg
class IdArgs {
    var id: String? = null
}

private class Failure(val code: String, message: String) : Exception(message)

/** One file being written: its row in the Downloads collection and the stream into it. */
private class Open(val uri: Uri, val stream: OutputStream, val name: String)

// Writes the page's file into Downloads in pieces. The row stays pending until `finish`,
// so a half file never shows in Files. Design: references/save-file.md.
@TauriPlugin
class SaveFilePlugin(private val activity: Activity) : Plugin(activity) {
    private val open = HashMap<String, Open>()
    private var counter = 0L

    @Command
    fun available(invoke: Invoke) = guarded(invoke) {
        invoke.resolve(JSObject().apply { put("place", PLACE) })
    }

    @Command
    fun begin(invoke: Invoke) = guarded(invoke) {
        val args = invoke.parseArgs(BeginArgs::class.java)
        synchronized(open) {
            if (open.size >= SaveRules.MAX_OPEN) throw Failure("busy", "Too many saves are open at once.")
            val name = SaveRules.safeName(args.name)
            val byName = SaveRules.extensionOf(name)?.let { MimeTypeMap.getSingleton().getMimeTypeFromExtension(it) }
            val values = ContentValues().apply {
                put(MediaStore.Downloads.DISPLAY_NAME, name)
                put(MediaStore.Downloads.MIME_TYPE, SaveRules.storedMime(byName))
                put(MediaStore.Downloads.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
                put(MediaStore.Downloads.IS_PENDING, 1)
            }
            val resolver = activity.contentResolver
            val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
                ?: throw Failure("unwritable", "The device would not make a file in Downloads.")
            val stream = try {
                resolver.openOutputStream(uri) ?: throw Failure("unwritable", "The device would not open the new file.")
            } catch (e: Exception) {
                resolver.delete(uri, null, null)
                throw e
            }
            // Only a guess: a taken name is numbered when the row stops being pending.
            val kept = resolver.query(uri, arrayOf(MediaStore.Downloads.DISPLAY_NAME), null, null, null)?.use {
                if (it.moveToFirst()) it.getString(0) else null
            } ?: SaveRules.safeName(args.name)
            val id = (++counter).toString()
            open[id] = Open(uri, stream, kept)
            invoke.resolve(JSObject().apply {
                put("id", id)
                put("name", kept)
            })
        }
    }

    @Command
    fun write(invoke: Invoke) = guarded(invoke) {
        val args = invoke.parseArgs(WriteArgs::class.java)
        val bytes = try {
            java.util.Base64.getDecoder().decode(args.data ?: "")
        } catch (e: IllegalArgumentException) {
            throw Failure("bad_data", "A piece of the file was not valid.")
        }
        val file = synchronized(open) { open[args.id ?: ""] } ?: throw Failure("unknown_save", "That save is not open.")
        file.stream.write(bytes)
        invoke.resolve()
    }

    @Command
    fun finish(invoke: Invoke) = guarded(invoke) {
        val file = synchronized(open) { open.remove(invoke.parseArgs(IdArgs::class.java).id ?: "") }
            ?: throw Failure("unknown_save", "That save is not open.")
        file.stream.close()
        val done = ContentValues().apply { put(MediaStore.Downloads.IS_PENDING, 0) }
        val resolver = activity.contentResolver
        resolver.update(file.uri, done, null, null)
        // A taken name is numbered when the row stops being pending, so read it now.
        val kept = resolver.query(file.uri, arrayOf(MediaStore.Downloads.DISPLAY_NAME), null, null, null)?.use {
            if (it.moveToFirst()) it.getString(0) else null
        } ?: file.name
        invoke.resolve(JSObject().apply {
            put("name", kept)
            put("place", PLACE)
        })
    }

    @Command
    fun cancel(invoke: Invoke) = guarded(invoke) {
        val file = synchronized(open) { open.remove(invoke.parseArgs(IdArgs::class.java).id ?: "") }
        if (file != null) {
            try {
                file.stream.close()
            } finally {
                activity.contentResolver.delete(file.uri, null, null)
            }
        }
        invoke.resolve()
    }

    private fun guarded(invoke: Invoke, body: () -> Unit) {
        try {
            body()
        } catch (e: Failure) {
            invoke.reject(e.message, e.code, e)
        } catch (e: Exception) {
            invoke.reject(e.message ?: "unexpected failure", "unwritable", e)
        }
    }

    private companion object {
        const val PLACE = "your Downloads folder"
    }
}
