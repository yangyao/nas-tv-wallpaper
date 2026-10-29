package com.gathin.photoframe;

import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.RectF;

/** A transient, low-resolution backdrop. Created on the display decoder thread. */
final class PhotoBackdrop {
    private static final int WIDTH = 128, HEIGHT = 72, RADIUS = 6;

    static Bitmap create(Bitmap original) {
        Bitmap result = Bitmap.createBitmap(WIDTH, HEIGHT, Bitmap.Config.ARGB_8888);
        Canvas canvas = new Canvas(result);
        float scale = Math.max((float) WIDTH / original.getWidth(),
                (float) HEIGHT / original.getHeight());
        float width = original.getWidth() * scale, height = original.getHeight() * scale;
        canvas.drawBitmap(original, null, new RectF((WIDTH-width)/2, (HEIGHT-height)/2,
                (WIDTH+width)/2, (HEIGHT+height)/2), new Paint(Paint.FILTER_BITMAP_FLAG));

        int[] pixels = new int[WIDTH * HEIGHT], scratch = new int[pixels.length];
        result.getPixels(pixels, 0, WIDTH, 0, 0, WIDTH, HEIGHT);
        // Three separable box passes approximate a Gaussian. Clamp edges so the
        // fill never grows black margins. Work and memory are independent of source size.
        for (int pass = 0; pass < 3; pass++) {
            blur(pixels, scratch, WIDTH, HEIGHT, true);
            blur(scratch, pixels, WIDTH, HEIGHT, false);
        }
        for (int i = 0; i < pixels.length; i++) {
            int p = pixels[i];
            float r = Color.red(p), g = Color.green(p), b = Color.blue(p);
            float gray = .2126f*r + .7152f*g + .0722f*b;
            pixels[i] = Color.rgb(Math.round((gray + (r-gray)*.65f)*.72f),
                    Math.round((gray + (g-gray)*.65f)*.72f),
                    Math.round((gray + (b-gray)*.65f)*.72f));
        }
        result.setPixels(pixels, 0, WIDTH, 0, 0, WIDTH, HEIGHT);
        return result;
    }

    private static void blur(int[] input, int[] output, int width, int height, boolean horizontal) {
        int lines = horizontal ? height : width, length = horizontal ? width : height;
        int span = RADIUS * 2 + 1;
        for (int line = 0; line < lines; line++) {
            int red = 0, green = 0, blue = 0;
            for (int k = -RADIUS; k <= RADIUS; k++) {
                int offset = Math.max(0, Math.min(length-1, k));
                int p = input[horizontal ? line*width+offset : offset*width+line];
                red += Color.red(p); green += Color.green(p); blue += Color.blue(p);
            }
            for (int at = 0; at < length; at++) {
                output[horizontal ? line*width+at : at*width+line] = Color.rgb(red/span, green/span, blue/span);
                int remove = Math.max(0, at-RADIUS), add = Math.min(length-1, at+RADIUS+1);
                int a = input[horizontal ? line*width+remove : remove*width+line];
                int b = input[horizontal ? line*width+add : add*width+line];
                red += Color.red(b)-Color.red(a);
                green += Color.green(b)-Color.green(a);
                blue += Color.blue(b)-Color.blue(a);
            }
        }
    }
}
