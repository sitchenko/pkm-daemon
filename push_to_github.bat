@echo off
echo [INFO] Committing and Pushing to GitHub...

git add .
git commit -m "fix: custom system tray, autostart quotes, hide console"
git push

echo [INFO] Done.
pause
