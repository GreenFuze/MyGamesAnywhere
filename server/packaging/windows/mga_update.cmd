@echo off
setlocal
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "%~dp0mga_update.ps1" %*
exit /b %ERRORLEVEL%
