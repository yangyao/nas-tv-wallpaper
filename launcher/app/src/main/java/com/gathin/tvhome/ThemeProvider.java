package com.gathin.tvhome;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;

import java.io.File;
import java.io.FileNotFoundException;

/**
 * Lets tools/theme.sh write the theme files into {@link Config#dir} on firmware where the adb shell is
 * locked out of Android/data (TCL G07, Android 11). Only the files {@link ThemeFiles} allows, and only for
 * callers holding WRITE_SECURE_SETTINGS, which the adb shell has and ordinary apps cannot get.
 *
 *   content write --uri content://com.gathin.tvhome.theme/config.json < /data/local/tmp/config.json
 *   content write --uri content://com.gathin.tvhome.theme/wallpapers/dusk.jpg < /data/local/tmp/dusk.jpg
 *   content delete --uri content://com.gathin.tvhome.theme/config.json
 *   content delete --uri content://com.gathin.tvhome.theme/wallpapers      (every image in the collection)
 */
public class ThemeProvider extends ContentProvider {

    @Override
    public android.os.Bundle call(String method, String arg, android.os.Bundle extras) {
        if (android.os.Binder.getCallingUid() != android.os.Process.myUid()
                && getContext().checkCallingPermission("android.permission.WRITE_SECURE_SETTINGS")
                != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            throw new SecurityException("shell permission required");
        }
        android.os.Bundle out = new android.os.Bundle();
        if ("nas-disable".equals(method) || "nas-enable".equals(method)) {
            boolean enabled = "nas-enable".equals(method);
            if (enabled && !NasPhotoSettings.load(getContext()).configured()) {
                out.putString("status", "not configured"); return out;
            }
            NasPhotoSettings.prefs(getContext()).edit().putBoolean("enabled", enabled).commit();
            NasPhotoSettings.changed(getContext());
            out.putString("status", enabled ? "enabled" : "disabled"); return out;
        }
        if ("nas-status".equals(method)) {
            java.io.File root = NasPhotoCache.root(getContext());
            org.json.JSONObject state = NasPhotoCache.json(new java.io.File(root, "state.json"));
            org.json.JSONObject pending = NasPhotoCache.json(new java.io.File(root, "pending.json"));
            out.putBoolean("enabled", NasPhotoSettings.load(getContext()).enabled);
            out.putInt("cached", NasPhotoCache.batchFiles(getContext(), state.optString("current")).size());
            out.putInt("pending", NasPhotoCache.batchFiles(getContext(), pending.optString("directory")).size());
            out.putLong("bytes", NasPhotoCache.size(root));
            out.putLong("last_success", state.optLong("success", 0));
            out.putString("status", NasPhotoSettings.prefs(getContext()).getString("status", "not downloaded"));
            return out;
        }
        if ("nas-refresh".equals(method)) {
            NasPhotoCache.refreshNextHome(getContext());
            NasPhotoSettings.changed(getContext());
            out.putString("status", "scheduled"); return out;
        }
        return super.call(method, arg, extras);
    }

    private File file(Uri uri) throws FileNotFoundException {
        String path = ThemeFiles.path(uri.getPathSegments());
        File dir = Config.dir(getContext());
        if (dir == null || path == null) throw new FileNotFoundException(String.valueOf(uri));
        File f = new File(dir, path);
        File parent = f.getParentFile();
        if (parent != null && !parent.isDirectory() && !parent.mkdirs()) throw new FileNotFoundException(String.valueOf(uri));
        return f;
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        File f = file(uri);
        if (mode.contains("w")) {
            return ParcelFileDescriptor.open(f, ParcelFileDescriptor.MODE_WRITE_ONLY
                    | ParcelFileDescriptor.MODE_CREATE | ParcelFileDescriptor.MODE_TRUNCATE);
        }
        return ParcelFileDescriptor.open(f, ParcelFileDescriptor.MODE_READ_ONLY);
    }

    @Override
    public int delete(Uri uri, String selection, String[] selectionArgs) {
        File dir = Config.dir(getContext());
        if (dir != null && ThemeFiles.isCollection(uri.getPathSegments())) {
            File[] images = new File(dir, ThemeFiles.WALLPAPERS).listFiles((d, name) -> ThemeFiles.collectionImage(name));
            int n = 0;
            if (images != null) for (File f : images) if (f.delete()) n++;
            return n;
        }
        try {
            return file(uri).delete() ? 1 : 0;
        } catch (FileNotFoundException e) {
            return 0;
        }
    }

    @Override public boolean onCreate() { return true; }
    @Override public String getType(Uri uri) { return null; }
    @Override public Cursor query(Uri uri, String[] p, String s, String[] a, String o) { return null; }
    @Override public Uri insert(Uri uri, ContentValues values) { return null; }
    @Override public int update(Uri uri, ContentValues v, String s, String[] a) { return 0; }
}
