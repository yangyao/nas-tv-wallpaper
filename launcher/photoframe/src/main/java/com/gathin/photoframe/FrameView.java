package com.gathin.photoframe;

import android.content.Context;
import android.graphics.*;
import android.os.SystemClock;
import android.view.View;
import android.view.animation.OvershootInterpolator;
import java.util.*;

/** Photo first; a small preblurred backdrop keeps the glass cheap on TV GPUs. */
final class FrameView extends View {
    private final Paint paint=new Paint(Paint.ANTI_ALIAS_FLAG|Paint.FILTER_BITMAP_FLAG);
    private final RectF bounds=new RectF(),panel=new RectF(),thumbBounds=new RectF();
    private final Matrix shaderMatrix=new Matrix();
    private final Map<Bitmap,BitmapShader> thumbShaders=new IdentityHashMap<>();
    private final LinearGradient sheen=new LinearGradient(0,0,0,1,new int[]{0x30FFFFFF,0x10FFFFFF,0x04FFFFFF},null,Shader.TileMode.CLAMP);
    private final LinearGradient rim=new LinearGradient(0,0,1,1,0x78FFFFFF,0x24FFFFFF,Shader.TileMode.CLAMP);
    private final boolean animate;
    private Bitmap current,previous,backdrop,previousBackdrop;
    private List<FrameSession.Photo> photos=Collections.emptyList();
    private int position;
    private long changed,railChanged,motionChanged;
    private float focusFrom,focusTarget,scrollFrom,scrollTarget;
    private static final long SELECTION_MS=360;
    private final OvershootInterpolator focusEase=new OvershootInterpolator(.55f);
    private float railFrom,railTarget;
    private boolean rail;
    private final Runnable hide=()->rail(false);
    public FrameView(Context c){this(c,true);}
    FrameView(Context c,boolean animate){super(c);this.animate=animate;setFocusable(true);setContentDescription("照片");}
    boolean railVisible(){return rail;}
    void items(List<FrameSession.Photo> items,int selected){
        long now=SystemClock.uptimeMillis();
        float focus=focusAt(now),scroll=scrollAt(now);
        int shift=0;
        // Appending/evicting cache entries must not move the visible photograph.
        if(!photos.isEmpty()&&position<photos.size()){
            FrameSession.Photo anchor=photos.get(position);
            for(int i=0;i<items.size();i++)if(items.get(i)==anchor){shift=i-position;break;}
        }
        float nextScroll=Math.max(0,Math.min(selected-3,items.size()-7));
        boolean move=selected!=focusTarget+shift||nextScroll!=scrollTarget+shift;
        if(animate&&rail&&!photos.isEmpty()&&move){
            focusFrom=focus+shift;scrollFrom=scroll+shift;
            focusTarget=selected;scrollTarget=nextScroll;motionChanged=now;
        }else if(!move){
            focusFrom+=shift;focusTarget+=shift;scrollFrom+=shift;scrollTarget+=shift;
        }else{
            focusFrom=focusTarget=selected;scrollFrom=scrollTarget=nextScroll;motionChanged=0;
        }
        photos=items;position=selected;
        Set<Bitmap> keep=Collections.newSetFromMap(new IdentityHashMap<>());
        for(FrameSession.Photo p:items)if(p.thumb!=null){keep.add(p.thumb);if(!thumbShaders.containsKey(p.thumb))thumbShaders.put(p.thumb,new BitmapShader(p.thumb,Shader.TileMode.CLAMP,Shader.TileMode.CLAMP));}
        thumbShaders.keySet().retainAll(keep);invalidate();
    }
    private float motionProgress(long now){return animate?Math.min(1f,(now-motionChanged)/(float)SELECTION_MS):1f;}
    private float focusAt(long now){float t=motionProgress(now);return focusFrom+(focusTarget-focusFrom)*focusEase.getInterpolation(t);}
    private float scrollAt(long now){float t=motionProgress(now);t=1-(1-t)*(1-t)*(1-t);return scrollFrom+(scrollTarget-scrollFrom)*t;}
    void image(Bitmap bitmap,Bitmap background){
        if(previous!=null)previous.recycle();
        if(previousBackdrop!=null)previousBackdrop.recycle();
        previous=current;previousBackdrop=backdrop;
        current=bitmap;backdrop=background;changed=SystemClock.uptimeMillis();
        invalidate();
    }
    private void scene(Canvas canvas,Bitmap photograph,Bitmap background){
        paint.setShader(null);paint.setStyle(Paint.Style.FILL);paint.setAlpha(255);
        bounds.set(0,0,getWidth(),getHeight());
        if(background!=null)canvas.drawBitmap(background,null,bounds,paint);
        photo(canvas,photograph,255);
    }
    void rail(boolean visible){railFrom=railAlpha();railTarget=visible?1:0;railChanged=SystemClock.uptimeMillis();rail=visible;removeCallbacks(hide);if(visible)postDelayed(hide,4500);invalidate();}
    private float railAlpha(){float t=animate?Math.min(1,(SystemClock.uptimeMillis()-railChanged)/220f):1;t=1-(1-t)*(1-t);return railFrom+(railTarget-railFrom)*t;}
    private float dp(float n){return n*getResources().getDisplayMetrics().density;}
    private void photo(Canvas c,Bitmap b,int alpha){if(b==null||b.isRecycled())return;float scale=Math.min((float)getWidth()/b.getWidth(),(float)getHeight()/b.getHeight());float w=b.getWidth()*scale,h=b.getHeight()*scale;bounds.set((getWidth()-w)/2,(getHeight()-h)/2,(getWidth()+w)/2,(getHeight()+h)/2);paint.setAlpha(alpha);c.drawBitmap(b,null,bounds,paint);paint.setAlpha(255);}
    @Override protected void onDraw(Canvas c){
        super.onDraw(c);if(current==null)return;
        float t=animate&&previous!=null?Math.min(1,(SystemClock.uptimeMillis()-changed)/320f):1;
        if(t<1){
            scene(c,previous,previousBackdrop);
            int transition=c.saveLayerAlpha(0,0,getWidth(),getHeight(),Math.round(255*t));
            scene(c,current,backdrop);c.restoreToCount(transition);postInvalidateOnAnimation();
        }else{
            scene(c,current,backdrop);
            if(previous!=null){previous.recycle();previous=null;}
            if(previousBackdrop!=null){previousBackdrop.recycle();previousBackdrop=null;}
        }
        float alpha=railAlpha();if(alpha<=0)return;
        float width=Math.min(getWidth()-dp(96),dp(780)),height=dp(108),bottom=getHeight()-dp(34)+(1-alpha)*dp(16);
        panel.set((getWidth()-width)/2,bottom-height,(getWidth()+width)/2,bottom);
        int layer=c.saveLayerAlpha(0,0,getWidth(),getHeight(),Math.round(alpha*255));
        // A light translucent pane: no artificial dark outer slab or shadow.
        paint.setShader(null);paint.setColor(0x34FFFFFF);
        c.drawRoundRect(panel,dp(26),dp(26),paint);
        shaderMatrix.setScale(panel.width(),panel.height());shaderMatrix.postTranslate(panel.left,panel.top);sheen.setLocalMatrix(shaderMatrix);rim.setLocalMatrix(shaderMatrix);paint.setShader(sheen);paint.setAlpha(255);c.drawRoundRect(panel,dp(26),dp(26),paint);paint.setShader(null);
        paint.setStyle(Paint.Style.STROKE);paint.setStrokeWidth(dp(1));paint.setShader(rim);c.drawRoundRect(panel,dp(26),dp(26),paint);paint.setShader(null);paint.setStyle(Paint.Style.FILL);
        int slots=Math.min(7,photos.size());
        float gap=dp(10),tw=Math.min(dp(92),(width-dp(40)-(Math.max(0,slots-1))*gap)/Math.max(1,slots)),th=dp(68);
        long now=SystemClock.uptimeMillis();
        float focus=Math.max(0,Math.min(photos.size()-1,focusAt(now))),scroll=scrollAt(now),step=tw+gap;
        float origin=panel.centerX()-(slots*tw+Math.max(0,slots-1)*gap)/2;
        int clipped=c.save();c.clipRect(panel.left+dp(12),panel.top,panel.right-dp(12),panel.bottom);
        int first=Math.max(0,(int)Math.floor(scroll)-1),last=Math.min(photos.size(),(int)Math.ceil(scroll)+slots+1);
        for(int i=first;i<last;i++){
            Bitmap thumb=photos.get(i).thumb;
            float emphasis=Math.max(0,1-Math.abs(i-focus)),x=origin+(i-scroll)*step;
            RectF r=thumbBounds;r.set(x,panel.centerY()-th/2,x+tw,panel.centerY()+th/2);r.inset(-dp(3)*emphasis,-dp(3)*emphasis);
            if(thumb!=null&&!thumb.isRecycled()){
                BitmapShader shader=thumbShaders.get(thumb);Matrix m=shaderMatrix;float s=Math.max(r.width()/thumb.getWidth(),r.height()/thumb.getHeight());m.setScale(s,s);m.postTranslate(r.centerX()-thumb.getWidth()*s/2,r.centerY()-thumb.getHeight()*s/2);shader.setLocalMatrix(m);paint.setShader(shader);paint.setAlpha(Math.round(200+55*emphasis));c.drawRoundRect(r,dp(10),dp(10),paint);paint.setShader(null);paint.setAlpha(255);
            }
        }
        if(slots>0){
            float x=origin+(focus-scroll)*step,stretch=dp(Math.min(4,Math.abs(focusTarget-focus)*2));
            thumbBounds.set(x-dp(6)-stretch,panel.centerY()-th/2-dp(6),x+tw+dp(6)+stretch,panel.centerY()+th/2+dp(6));
            paint.setStyle(Paint.Style.STROKE);paint.setStrokeWidth(dp(2));paint.setColor(0xF2FFFFFF);
            c.drawRoundRect(thumbBounds,dp(13),dp(13),paint);paint.setStyle(Paint.Style.FILL);
        }
        c.restoreToCount(clipped);
        if(motionProgress(now)<1)postInvalidateOnAnimation();
        c.restoreToCount(layer);if(Math.abs(alpha-railTarget)>.001f)postInvalidateOnAnimation();
    }
    void release(){removeCallbacks(hide);if(current!=null)current.recycle();if(previous!=null)previous.recycle();if(backdrop!=null)backdrop.recycle();if(previousBackdrop!=null)previousBackdrop.recycle();current=previous=backdrop=previousBackdrop=null;thumbShaders.clear();photos=Collections.emptyList();}
}
