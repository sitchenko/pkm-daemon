@echo off
chcp 65001 >nul

:: Запуск проекта
echo.
echo [INFO] Запускаем PKM Daemon...
echo --------------------------------------------------
go run .\cmd\pkm-daemon\main.go
echo --------------------------------------------------

pause