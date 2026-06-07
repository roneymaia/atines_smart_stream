# Atines Smart Stream

Ponte **RTSP → RTMP** (destino [SRS](https://github.com/ossrs/srs)) com UI web embutida.
Um binário único por plataforma, foco em **baixo consumo** e **facilidade para leigos**:
clicou no executável, a tela abre no navegador, e você gerencia as câmeras.

## Como funciona

Cada câmera vira uma "conversão". O app decide **por câmera** entre:

- **Remux** (copiar streams, CPU ~0) quando a câmera já entrega **H.264** — só reembala
  para RTMP/FLV.
- **Transcode** para H.264 quando a câmera usa codecs mais antigos (**MJPEG, MPEG-4,
  H.263**) ou **H.265**. O áudio é normalizado para **AAC** quando necessário (G.711 etc.).

A aceleração por hardware (NVENC / QuickSync / AMF / VAAPI) é detectada e usada
automaticamente quando há transcode; sem ela, cai para `libx264 -preset veryfast`.

## Como usar (usuário final)

1. Coloque `ffmpeg`/`ffprobe` (e `.exe` no Windows) na mesma pasta do executável,
   ou tenha-os no PATH.
2. Rode o executável. A tela de gerência abre no navegador (http://127.0.0.1:8787).
3. Cadastre câmeras (nome, URL RTSP, usuário/senha RTSP, URL RTMP de destino) e use o
   toggle **Ativo** para ligar/desligar cada conversão.

### Opções de linha de comando

| Flag | Descrição |
|---|---|
| `--addr 127.0.0.1:8787` | endereço de escuta da UI |
| `--data <caminho>` | arquivo de cadastro (`cameras.json`) |
| `--no-browser` | não abre o navegador ao iniciar |
| `--install-service` | instala como serviço do SO (roda 24/7 no boot) |
| `--uninstall-service` | remove o serviço |

As conversões rodam dentro do processo do app — **fechar a aba do navegador não para
nada**. Para operação 24/7 desacompanhada, instale o auto-start (veja abaixo).

## Rodar sempre (24/7, no startup)

Não há botão na interface para isso — o toggle "Ativo" da tela é **por câmera**. O
auto-start é via linha de comando, uma vez:

### Windows (como **Administrador**)
Abra o **Prompt de Comando** ou **PowerShell como Administrador**, vá até a pasta do
programa e:

```bat
atines-smart-stream-windows-amd64.exe --install-service     REM ativa (sobe no boot)
atines-smart-stream-windows-amd64.exe --uninstall-service   REM desativa
```

Isso cria uma **Tarefa Agendada** (`AtinesSmartStream`, gatilho "ao iniciar o sistema",
conta `SYSTEM`) e já a inicia. Confira/remova também pelo **Agendador de Tarefas** do
Windows (`taskschd.msc`). Depois é só abrir `http://127.0.0.1:8787` no navegador para
gerenciar — o programa roda em segundo plano.

> Por que Tarefa Agendada e não "Serviço" (`services.msc`)? Um serviço de verdade do
> Windows precisa falar o protocolo do SCM; este executável simples não fala, então
> `sc create` falharia (erro 1053). A Tarefa Agendada roda um `.exe` comum no boot de
> forma confiável.

### Linux (systemd do usuário)
```bash
./atines-smart-stream-linux-amd64 --install-service     # ativa
./atines-smart-stream-linux-amd64 --uninstall-service   # remove
```
Cria um serviço `--user` (`systemctl --user`). Para subir no boot **sem login**, habilite
o linger uma vez: `loginctl enable-linger $USER`.

## Build (desenvolvedor)

Requer **Go 1.22+**.

```bash
./build.sh   # gera bin/ para linux/windows x amd64/arm64
```

`CGO_ENABLED=0`: cross-compila sem dependências de C. A persistência é um arquivo JSON
(`cameras.json`, escrita atômica, permissão `0600`); a UI é embutida no binário via
`go:embed`. FFmpeg/ffprobe são empacotados no release ao lado do binário.

### Empacotar com FFmpeg incluído

`package.sh` baixa o `ffmpeg`/`ffprobe` estático certo por plataforma
(de [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds), GPL) e gera, em `dist/`,
um pacote autocontido por plataforma (binário + ffmpeg + ffprobe + licenças):

```bash
./package.sh                 # todos os alvos
./package.sh linux/amd64     # só um
```

Linux vira `.tar.gz`, Windows vira `.zip`. O download fica em cache (`vendor-ffmpeg/`).
Requer `curl`, `tar`/`xz`, `unzip` e `python3` (para criar o `.zip`).

> **Tamanho:** o ffmpeg estático "full" é grande (~150–200 MB cada `ffmpeg`/`ffprobe`),
> então o pacote fica ~150 MB por plataforma. É o preço de zero-instalação. Alternativas
> para encolher: usar um build "essentials"/menor, dropar o `ffprobe` empacotado (o app já
> tolera falha de probe), ou não empacotar e pedir ao usuário um ffmpeg do sistema
> (`winget install ffmpeg` / `apt install ffmpeg`). Como FFmpeg é **GPL**, o
> `FFMPEG-LICENSE.txt` vai junto no pacote.

### Testes

```bash
go test ./...          # suíte completa
go test ./... -race    # com detector de corrida (recomendado p/ o supervisor)
```

## Verificação fim-a-fim (com SRS real)

1. Suba um SRS: `docker run --rm -p 1935:1935 -p 8080:8080 ossrs/srs:5`
2. Aponte uma câmera real (ou um RTSP de teste) para o app e cadastre o destino
   `rtmp://<host-do-srs>/live/<chave>`.
3. Ative a câmera; o status deve ir para **rodando** e o stream aparecer no SRS.

## Arquitetura

Veja `docs/superpowers/specs/2026-06-06-atines-smart-stream-design.md` e o plano em
`docs/superpowers/plans/2026-06-06-atines-smart-stream.md`.

Componentes (em `internal/`): `model` (tipos), `store` (registro JSON), `media`
(estratégia de args + probe + capabilities), `supervisor` (ciclo de vida do FFmpeg por
câmera com backoff), `ffmpeg` (locator), `api` (HTTP + SSE), `webui` (UI embutida),
`launcher` (abre o navegador), `service` (instalação como serviço).
