#!/usr/bin/env python3
"""Toggle NAS photos or reinstall the baseline, without uninstalling the launcher."""
import argparse, pathlib, shutil, subprocess
p=argparse.ArgumentParser()
p.add_argument('action',choices=['on','off','refresh','status','rollback'])
p.add_argument('--tv',required=True)
p.add_argument('--adb',default=shutil.which('adb') or str(pathlib.Path.home()/'Library/Android/sdk/platform-tools/adb'))
p.add_argument('--apk',type=pathlib.Path,default=pathlib.Path(__file__).resolve().parents[1]/'private-backup/launcher-before.apk')
a=p.parse_args()
def run(*cmd):return subprocess.run([a.adb,*cmd],check=True)
run('connect',a.tv)
if a.action=='rollback':
 if not a.apk.is_file():p.error('Baseline APK missing; supply your own signed backup with --apk.')
 run('-s',a.tv,'install','-r',str(a.apk))
 run('-s',a.tv,'shell','am','start','-W','-n','com.gathin.tvhome/.MainActivity')
else:
 method={'on':'nas-enable','off':'nas-disable','refresh':'nas-refresh','status':'nas-status'}[a.action]
 run('-s',a.tv,'shell','content','call','--uri','content://com.gathin.tvhome.theme','--method',method)
