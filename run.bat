@echo off
chcp 65001 >nul

:: Start ngrok in a new window
echo [INFO] Starting ngrok on port 8080...
start "ngrok" "D:\3. programs\ngrok.exe" http 8080

echo [INFO] Waiting for ngrok to initialize...
timeout /t 3 /nobreak >nul

:: Start project
echo.
echo [INFO] Building and Starting PKM Daemon...
echo --------------------------------------------------
go build -o pkm-daemon.exe .\cmd\pkm-daemon\main.go
pkm-daemon.exe
echo --------------------------------------------------

pause