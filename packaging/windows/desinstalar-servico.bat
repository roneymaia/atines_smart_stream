@echo off
title Atines Smart Stream - Remover auto-start
REM Precisa de Administrador. Se nao estiver elevado, se re-lanca via UAC.
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Solicitando privilegios de administrador...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)
cd /d "%~dp0"
echo Removendo o auto-start do Atines Smart Stream...
echo.
"%~dp0atines-smart-stream.exe" --uninstall-service
echo.
echo Removido. O programa nao sobe mais sozinho no boot.
echo.
pause
