@echo off

:: Enable ANSI colors in Windows CMD
REG ADD HKCU\CONSOLE /f /v VirtualTerminalLevel /t REG_DWORD /d 1 >nul

echo ========================================================
echo [1/2] Installing gotestsum for beautiful test output...
echo ========================================================
go install gotest.tools/gotestsum@latest

echo.
echo ========================================================
echo [2/2] Running Unit Tests with Coverage...
echo ========================================================
call %USERPROFILE%\go\bin\gotestsum.exe --format testdox -- -v -cover ./...

echo.
echo ========================================================
echo DONE!
echo ========================================================
pause
