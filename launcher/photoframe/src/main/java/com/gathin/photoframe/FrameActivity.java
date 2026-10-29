package com.gathin.photoframe;

import android.app.Activity;
import android.graphics.Bitmap;
import android.os.*;
import android.view.*;
import java.io.File;
import java.util.*;

/** Internal activity hosted by Home: no extra icon or loading UI. */
public abstract class FrameActivity extends Activity {
    private FrameSession session;
    private FrameView frame;
    private final Handler handler=new Handler(Looper.getMainLooper());
    private boolean playing=true;
    protected abstract PhotoSource photoSource();
    protected abstract List<File> seedPhotos();
    protected boolean animationsEnabled(){return true;}
    private final Runnable advance=new Runnable(){public void run(){if(session!=null&&playing&&!frame.railVisible())session.move(1);handler.postDelayed(this,12000);}};
    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_FULLSCREEN|View.SYSTEM_UI_FLAG_HIDE_NAVIGATION|View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);frame=new FrameView(this,animationsEnabled());setContentView(frame);frame.requestFocus();}
    @Override protected void onStart(){super.onStart();session=new FrameSession(this,photoSource(),seedPhotos(),new FrameSession.Listener(){public void timeline(List<FrameSession.Photo> photos,int position){frame.items(photos,position);}public void image(Bitmap bitmap){frame.image(bitmap);}});handler.postDelayed(advance,12000);}
    @Override protected void onStop(){handler.removeCallbacks(advance);if(session!=null){session.close();session=null;}super.onStop();}
    @Override protected void onDestroy(){frame.release();super.onDestroy();}
    @Override public boolean onKeyDown(int key,KeyEvent event){
        switch(key){
            case KeyEvent.KEYCODE_DPAD_LEFT:case KeyEvent.KEYCODE_DPAD_RIGHT:
                handler.removeCallbacks(advance);handler.postDelayed(advance,12000);if(session!=null)session.move(key==KeyEvent.KEYCODE_DPAD_LEFT?-1:1);if(frame.railVisible())frame.rail(true);return true;
            case KeyEvent.KEYCODE_DPAD_DOWN:frame.rail(true);return true;
            case KeyEvent.KEYCODE_DPAD_UP:frame.rail(false);return true;
            case KeyEvent.KEYCODE_DPAD_CENTER:case KeyEvent.KEYCODE_ENTER:
                if(event.getRepeatCount()==0){if(frame.railVisible())frame.rail(false);else playing=!playing;}return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE:if(event.getRepeatCount()==0)playing=!playing;return true;
            case KeyEvent.KEYCODE_MEDIA_PLAY:playing=true;return true;
            case KeyEvent.KEYCODE_MEDIA_PAUSE:playing=false;return true;
            case KeyEvent.KEYCODE_BACK:if(event.getRepeatCount()==0)finish();return true;
            default:return super.onKeyDown(key,event);
        }
    }
}
