@echo off
setlocal
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0Start MGA.ps1" %*
exit /b %ERRORLEVEL%
