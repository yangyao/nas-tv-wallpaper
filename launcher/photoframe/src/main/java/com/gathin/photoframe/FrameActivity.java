package com.gathin.photoframe;

import android.app.Activity;
import android.graphics.Bitmap;
import android.os.*;
import android.view.*;
import java.io.File;
import java.util.*;

/** Internal activity hosted by Home: no extra icon or loading UI. */
public abstract class FrameActivity extends Activity {
    public static final String EXTRA_IDLE="com.gathin.photoframe.IDLE";
    private FrameSession session;
    private FrameView frame;
    private final Handler handler=new Handler(Looper.getMainLooper());
    private boolean playing=true;
    private static final long SLIDE_MS=10000;
    protected abstract PhotoSource photoSource();
    protected abstract List<File> seedPhotos();
    protected boolean animationsEnabled(){return true;}
    private final Runnable advance=new Runnable(){public void run(){if(session!=null&&playing&&!frame.railVisible())session.move(1);handler.postDelayed(this,SLIDE_MS);}};
    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN|View.SYSTEM_UI_FLAG_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);frame=new FrameView(this,animationsEnabled());setContentView(frame);frame.requestFocus();}
    @Override protected void onStart(){super.onStart();session=new FrameSession(this,photoSource(),seedPhotos(),new FrameSession.Listener(){public void timeline(List<FrameSession.Photo> photos,int position){frame.items(photos,position);}public void image(Bitmap bitmap,Bitmap backdrop){frame.image(bitmap,backdrop);restartPlaybackTimer();}});handler.postDelayed(advance,SLIDE_MS);}
    private void restartPlaybackTimer(){handler.removeCallbacks(advance);handler.postDelayed(advance,SLIDE_MS);}
    @Override protected void onStop(){handler.removeCallbacks(advance);if(session!=null){session.close();session=null;}super.onStop();}
    @Override protected void onDestroy(){frame.release();super.onDestroy();}
    @Override public boolean onKeyDown(int key,KeyEvent event){
        if(getIntent().getBooleanExtra(EXTRA_IDLE,false)&&
                (key==KeyEvent.KEYCODE_DPAD_LEFT||key==KeyEvent.KEYCODE_DPAD_RIGHT||
                 key==KeyEvent.KEYCODE_DPAD_UP||key==KeyEvent.KEYCODE_DPAD_DOWN||
                 key==KeyEvent.KEYCODE_DPAD_CENTER||key==KeyEvent.KEYCODE_ENTER||
                 key==KeyEvent.KEYCODE_BACK||key==KeyEvent.KEYCODE_MENU)){
            if(event.getRepeatCount()==0)finish();return true;
        }
        switch(key){
            case KeyEvent.KEYCODE_DPAD_LEFT:case KeyEvent.KEYCODE_DPAD_RIGHT:
                restartPlaybackTimer();if(session!=null)session.move(key==KeyEvent.KEYCODE_DPAD_LEFT?-1:1);if(frame.railVisible())frame.rail(true);return true;
            case KeyEvent.KEYCODE_DPAD_DOWN:frame.rail(true);return true;
            case KeyEvent.KEYCODE_DPAD_UP:frame.rail(false);return true;
            case KeyEvent.KEYCODE_DPAD_CENTER:case KeyEvent.KEYCODE_ENTER:
                if(event.getRepeatCount()==0){if(frame.railVisible())frame.rail(false);else {playing=!playing;restartPlaybackTimer();}}return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:if(event.getRepeatCount()==0){playing=!playing;restartPlaybackTimer();}return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY:playing=true;restartPlaybackTimer();return true;
            case KeyEvent.KEYCODE_MEDIA_PAUSE:playing=false;return true;
            case KeyEvent.KEYCODE_BACK:if(event.getRepeatCount()==0)finish();return true;
            default:return super.onKeyDown(key,event);
        }
    }
}
