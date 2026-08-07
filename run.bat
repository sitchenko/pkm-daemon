@echo off
chcp 65001 >nul

echo [INFO] Building PKM Daemon...
go build -ldflags="-H windowsgui" -o pkm-daemon.exe .\cmd\pkm-daemon

if %ERRORLEVEL% neq 0 (
    echo [ERROR] Build failed!
    pause
    exit /b 1
)

echo [INFO] Starting PKM Daemon...
echo --------------------------------------------------
pkm-daemon.exe
echo --------------------------------------------------

pause