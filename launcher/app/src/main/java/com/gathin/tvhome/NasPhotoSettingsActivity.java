package com.gathin.tvhome;
import android.app.Activity;
import android.os.Bundle;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.text.InputType;
import android.view.*;
import android.view.inputmethod.InputMethodManager;
import android.widget.*;

/** User-facing off switch; no ADB required to restore the original background. */
public class NasPhotoSettingsActivity extends Activity {
 Switch enabled;EditText url,token;TextView status;boolean binding;
 int dp(int v){return Math.round(v*getResources().getDisplayMetrics().density);}
 @Override protected void onCreate(Bundle b){super.onCreate(b);getWindow().getDecorView().setSystemUiVisibility(5894);ScrollView scroll=new ScrollView(this);LinearLayout root=new LinearLayout(this);root.setOrientation(LinearLayout.VERTICAL);root.setPadding(dp(50),dp(25),dp(50),dp(30));root.setBackgroundColor(0xff101418);scroll.addView(root);setContentView(scroll);add(root,text("NAS 照片背景",27),50);add(root,text("每 2 分钟换一张；6 小时后回到首页时更新一批。关闭即可恢复原背景。",17),52);
 NasPhotoSettings config=NasPhotoSettings.load(this);enabled=new Switch(this);enabled.setText("启用 NAS 照片背景");enabled.setTextSize(22);enabled.setTextColor(Color.WHITE);enabled.setChecked(config.enabled);add(root,enabled,56);status=text(NasPhotoSettings.prefs(this).getString("status","尚未下载照片；关闭时不会连接 NAS"),17);add(root,status,42);add(root,text("NAS 服务地址",16),30);url=field(config.base,"http://192.168.x.x:8791",InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_URI);add(root,url,50);add(root,text("访问密钥",16),30);token=field(config.token,"NAS 服务访问密钥",InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);add(root,token,50);
 Switch frameMode=new Switch(this);frameMode.setText("桌面按返回进入相框");frameMode.setTextSize(22);frameMode.setTextColor(Color.WHITE);frameMode.setChecked(NasPhotoSettings.prefs(this).getBoolean("frame_enabled",true));add(root,frameMode,56);frameMode.setOnCheckedChangeListener((v,on)->NasPhotoSettings.prefs(this).edit().putBoolean("frame_enabled",on).apply());
 TextView save=button("保存连接设置",()->{if(saveConnection(enabled.isChecked()))Toast.makeText(this,"已保存，回到首页后生效",Toast.LENGTH_SHORT).show();});add(root,save,52);add(root,button("下一次回首页立即换一批",()->{NasPhotoCache.refreshNextHome(this);status.setText("已安排更新，回到首页后开始；当前缓存继续显示");}),52);add(root,button("关闭并恢复原背景",()->{NasPhotoSettings.prefs(this).edit().putBoolean("enabled",false).commit();NasPhotoSettings.changed(this);setResult(RESULT_OK);finish();}),52);enabled.setOnCheckedChangeListener((v,checked)->{if(binding)return;if(!saveConnection(checked)){binding=true;enabled.setChecked(false);binding=false;}});link(url,enabled,token);link(token,url,save);enabled.requestFocus();}
 TextView text(String s,int size){TextView t=new TextView(this);t.setText(s);t.setTextColor(0xffE4E8EB);t.setTextSize(size);t.setGravity(Gravity.CENTER_VERTICAL);return t;}
 EditText field(String value,String hint,int type){EditText e=new EditText(this);e.setSingleLine();e.setText(value);e.setHint(hint);e.setTextColor(Color.WHITE);e.setHintTextColor(0xff8D9BA7);e.setTextSize(18);e.setInputType(type);e.setShowSoftInputOnFocus(false);e.setOnClickListener(v->((InputMethodManager)getSystemService(INPUT_METHOD_SERVICE)).showSoftInput(e,InputMethodManager.SHOW_IMPLICIT));return e;}
 void link(EditText e,View up,View down){e.setOnKeyListener((v,key,event)->{if(event.getAction()!=KeyEvent.ACTION_DOWN)return false;if(key==20){down.requestFocus();return true;}if(key==19){up.requestFocus();return true;}if(key==23||key==66){e.performClick();return true;}return false;});}
 TextView button(String label,Runnable action){TextView t=text(label,19);t.setFocusable(true);t.setClickable(true);t.setPadding(dp(14),0,dp(14),0);t.setBackground(shape(false));t.setOnFocusChangeListener((v,focus)->t.setBackground(shape(focus)));t.setOnClickListener(v->action.run());return t;}
 GradientDrawable shape(boolean focus){GradientDrawable d=new GradientDrawable();d.setColor(focus?0xff33444F:0xff202A31);d.setCornerRadius(dp(10));d.setStroke(dp(focus?2:1),focus?Color.WHITE:0xff53616B);return d;}
 void add(LinearLayout root,View v,int h){LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,dp(h));p.bottomMargin=dp(9);root.addView(v,p);}
 boolean saveConnection(boolean on){String base=url.getText().toString().trim().replaceAll("/+$",""),secret=token.getText().toString().trim();if(on&&(!NasPhotoSettings.validURL(base)||secret.length()<24)){status.setText("请填写局域网 NAS 地址，以及至少 24 个字符的访问密钥");return false;}NasPhotoSettings.prefs(this).edit().putString("url",base).putString("token",secret).putBoolean("enabled",on).remove("failed").commit();NasPhotoSettings.changed(this);status.setText(on?"已启用，回到首页后检查更新":"已关闭，回到首页恢复原背景");setResult(RESULT_OK);return true;}
}
