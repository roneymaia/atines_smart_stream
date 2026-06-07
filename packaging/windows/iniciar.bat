@echo off
title Atines Smart Stream
cd /d "%~dp0"
echo ============================================================
echo  Atines Smart Stream
echo ============================================================
echo.

REM Arquivos baixados da internet vem "bloqueados" pelo Windows
REM (Mark of the Web). Isso costuma causar o erro "O Windows nao pode
REM acessar o dispositivo, caminho ou arquivo especificado".
REM Desbloqueia TUDO desta pasta (exe, ffmpeg.exe, .bat...).
echo [1/2] Desbloqueando os arquivos (vindos da internet)...
powershell -NoProfile -ExecutionPolicy Bypass -Command "Get-ChildItem -LiteralPath '%~dp0' -Recurse | Unblock-File" >nul 2>&1

if not exist "%~dp0atines-smart-stream.exe" (
    echo.
    echo  ERRO: nao encontrei "atines-smart-stream.exe" nesta pasta.
    echo  Extraia o .zip por completo e rode este arquivo de DENTRO da pasta.
    echo.
    pause
    exit /b 1
)

echo [2/2] Iniciando... a tela de gerencia abre no navegador:
echo       http://127.0.0.1:8787
echo.
echo  Deixe esta janela aberta enquanto estiver usando o programa.
echo  (para rodar 24/7 sozinho no boot, use o "instalar-servico.bat")
echo.
"%~dp0atines-smart-stream.exe"

echo.
echo ============================================================
echo  O programa foi encerrado.
echo.
echo  Se a janela fechou na hora, ou apareceu "acesso negado":
echo   - O antivirus / Seguranca do Windows pode ter bloqueado o
echo     arquivo. Adicione esta pasta como EXCECAO no Seguranca do
echo     Windows (Protecao contra virus -^> Exclusoes), ou restaure
echo     o arquivo da quarentena.
echo   - Veja "Problemas ao abrir no Windows" no README.md.
echo ============================================================
pause
