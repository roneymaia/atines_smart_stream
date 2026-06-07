@echo off
title Atines Smart Stream - Instalar auto-start
REM Precisa de Administrador. Se nao estiver elevado, se re-lanca via UAC.
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Solicitando privilegios de administrador...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)
cd /d "%~dp0"
echo Instalando o Atines Smart Stream para iniciar junto com o Windows...
echo.
"%~dp0atines-smart-stream.exe" --install-service
echo.
echo ============================================================
echo  Pronto! O programa sobe sozinho quando o Windows iniciar.
echo  Para gerenciar, abra no navegador:  http://127.0.0.1:8787
echo.
echo  NAO mova nem apague esta pasta - o auto-start aponta pra ela.
echo ============================================================
echo.
pause
