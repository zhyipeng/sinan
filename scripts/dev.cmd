@echo off
setlocal
rem Windows dev launcher. Requires PowerShell 7 (pwsh), never Windows PowerShell.
set "PS="
where pwsh.exe >nul 2>nul && set "PS=pwsh.exe"
if not defined PS if exist "%ProgramFiles%\PowerShell\7\pwsh.exe" set "PS=%ProgramFiles%\PowerShell\7\pwsh.exe"
if not defined PS (
  echo [dev] PowerShell 7 ^(pwsh^) not found. Install: winget install Microsoft.PowerShell
  exit /b 1
)
"%PS%" -NoLogo -ExecutionPolicy Bypass -File "%~dp0dev.ps1" %*
exit /b %errorlevel%
