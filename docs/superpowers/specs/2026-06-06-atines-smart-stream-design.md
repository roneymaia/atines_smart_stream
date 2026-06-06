# Atines Smart Stream — Design

**Data:** 2026-06-06
**Status:** Aprovado (brainstorming)

## 1. Objetivo

Aplicativo para **gerenciar conversores de câmeras** que fazem uma **ponte RTSP → RTMP**.
Cada câmera cadastrada vira uma "conversão": o app puxa o stream RTSP da câmera e
republica como RTMP num servidor de destino (um **SRS** rodando em Docker).

Foco do produto:
- **Menor consumo de CPU/RAM possível.**
- **Fácil para leigos:** clicou no executável (`.exe` no Windows / binário no Linux),
  abriu a tela, e gerencia. Sem instalação nem configuração de ambiente.
- **Multiplataforma:** Windows e Linux, em **ARM e x86-64**.

## 2. Requisitos

### Funcionais
- Cadastro de câmera com: **nome**, **URL RTSP** (ingest), **usuário** e **senha** RTSP,
  **URL RTMP** de destino (push completo, ex.: `rtmp://srs/live/cam1`).
- **Ativar/desativar** cada câmera individualmente (toggle).
- Listar câmeras com **status ao vivo** (rodando / parada / erro / reconectando).
- CRUD completo (criar, editar, excluir câmera).

### Não-funcionais
- Binário **único, sem dependências** (UI embutida). FFmpeg empacotado junto.
- Cross-compila para `windows/amd64`, `windows/arm64`, `linux/amd64`, `linux/arm64`.
- Gerenciador ocioso: ~10–20 MB RAM. CPU só sobe quando há **transcode**.
- Operação **24/7** opcional (modo serviço), além do modo "abrir e usar".

## 3. Decisão central: remux vs transcode

O custo de CPU é dominado pela mídia, não pela linguagem. A regra, decidida **por câmera**
após sondar os codecs com `ffprobe`:

| Vídeo de entrada | Estratégia de vídeo | Custo |
|---|---|---|
| **H.264** | `copy` (remux) | CPU ~0 |
| **H.265 (HEVC)** | transcode → H.264 (default, compat. SRS/FLV) | alto |
| **MJPEG / MPEG-4 Part 2 / H.263** | transcode → H.264 | alto |

| Áudio de entrada | Estratégia de áudio |
|---|---|
| **AAC** | `copy` |
| **G.711 / G.726 / outro** | re-encode → AAC 128 kbps (barato) |
| **sem áudio** | `-an` |

Justificativa: RTMP/FLV (e o SRS no caminho padrão) esperam **H.264 + AAC**. Câmeras
antigas usam codecs anteriores ao H.264 (MJPEG, MPEG-4, H.263) e áudio G.711, que não
entram "copiados".

**Override por câmera:** `auto` (default) | `copy` forçado | `transcode` forçado |
`manter H.265` (avançado, enhanced-RTMP).

### Aceleração por hardware
No boot, o app inspeciona os encoders disponíveis no FFmpeg (`ffmpeg -encoders`) e escolhe,
nesta ordem de preferência quando há transcode: `h264_nvenc` (NVIDIA) → `h264_qsv` (Intel
QuickSync) → `h264_amf` (AMD, Windows) → `h264_vaapi` (Linux) → **fallback** `libx264
-preset veryfast`. Como o hardware "varia bastante", a seleção é automática, com override
global/por-câmera: `auto | hardware | software`.

### Comandos FFmpeg de referência
Remux (câmera moderna H.264):
```
ffmpeg -rtsp_transport tcp -rw_timeout 5000000 -i rtsp://user:pass@cam/stream \
  -c:v copy -c:a aac -b:a 128k -f flv rtmp://srs/live/cam1
```
Transcode (câmera antiga, software):
```
ffmpeg -rtsp_transport tcp -rw_timeout 5000000 -i rtsp://user:pass@cam/stream \
  -c:v libx264 -preset veryfast -tune zerolatency -g 50 \
  -c:a aac -b:a 128k -f flv rtmp://srs/live/cam1
```
(Com accel, `libx264` é substituído pelo encoder de hardware escolhido.)

## 4. Arquitetura

Binário Go único com componentes internos bem isolados:

1. **Registry / Store** — persiste as câmeras em `cameras.json` (escrita atômica, perm
   `0600`). Campos: `id, nome, rtsp_url, usuario, senha, rtmp_url, ativo, overrides
   (transcode_mode, accel_mode), codecs_detectados (cache)`. Estado de runtime (status,
   uptime, último erro) fica **em memória**.
2. **Media Strategy** — sonda com `ffprobe`, decide os args do FFmpeg (copy vs transcode,
   seleção de encoder de hardware). Função pura testável: `(probe, overrides, capabilities)
   → []string` (argumentos).
3. **Supervisor** — uma goroutine por câmera ativa, gerencia o processo FFmpeg
   (`os/exec`): start/stop/restart. Reinício com **backoff exponencial** (teto ~30s) quando
   o processo morre e a câmera continua ativa. Faz parse do stderr do FFmpeg para detectar
   saúde (fps/bitrate) e erros. Publica status.
4. **HTTP API + UI** — API JSON (CRUD + toggle) e **SSE** para status ao vivo. Serve a SPA
   embutida via `go:embed`. Escuta em `127.0.0.1:8787` por padrão.
5. **Launcher** — ao subir: reserva a porta, **abre o navegador** na tela de gerência,
   religa as câmeras que estavam `ativo`. Expõe `--install-service` / `--uninstall-service`
   (systemd no Linux; Service API no Windows) para o modo 24/7.
6. **FFmpeg Locator** — procura `ffmpeg`/`ffprobe` ao lado do binário; fallback para o PATH.

### Fluxo de dados
```
UI (browser) ──HTTP/JSON──▶ API ──▶ Registry (cameras.json)
     ▲                       │
     └────────SSE────────── Supervisor ──spawn──▶ FFmpeg ──RTMP──▶ SRS
                              ▲                        │
                              └──── parse stderr ──────┘
```

## 5. Modo de execução (híbrido)

- As conversões rodam **dentro do processo Go**, não no navegador. Fechar a aba não para
  nada; só encerrar o binário para.
- Default: "abrir e usar". Botão/CLI opcional **instala como serviço** do SO para subir no
  boot e rodar 24/7 desacompanhado.

## 6. Resiliência

- Restart automático de FFmpeg com backoff quando o stream cai (o Supervisor reinicia, não
  o FFmpeg).
- `-rw_timeout` detecta stream morto.
- Ao iniciar o app, religa automaticamente toda câmera marcada como `ativo`.
- Status por câmera: `parada | iniciando | rodando | reconectando | erro` + última mensagem
  de erro + uptime.

## 7. UI

- SPA leve em **JS puro** (sem toolchain Node), CSS embutido, assets via `go:embed`.
- Tela única: lista de câmeras (cards/linhas) com nome, destino, **status colorido**,
  **toggle ativar/desativar**, e ações editar/excluir. Formulário de cadastro/edição.
- Atualização de status via SSE (sem polling pesado).
- Pensada para leigo: poucos campos, mensagens de erro claras em português.

## 8. Segurança

- Bind em `127.0.0.1` por padrão → sem login.
- Credenciais RTSP no `cameras.json` com permissão `0600`. Criptografia em repouso fica como
  evolução futura.
- Modo rede (`0.0.0.0` + senha simples) como opção de configuração futura.

## 9. Distribuição / Build

- `go build` com `go:embed` para os assets → 1 binário por alvo.
- Cross-compile via `GOOS`/`GOARCH` (CGO desligado: persistência é JSON, sem dependência C).
- Release empacota o binário + `ffmpeg`/`ffprobe` da plataforma na mesma pasta.

## 10. Estratégia de testes

- **Media Strategy**: testes de unidade da função de decisão (tabela de codecs/overrides/
  capabilities → args esperados). É o núcleo de risco e é determinística.
- **Registry**: testes de round-trip (load/save atômico, permissões).
- **Supervisor**: testes com um "fake ffmpeg" (script/binário stub que simula
  saída/erro/saída prematura) para validar backoff e transições de status.
- **API**: testes de handlers (CRUD, toggle) com store em memória.
- Teste manual fim-a-fim: uma câmera RTSP real (ou `ffmpeg` gerando um RTSP de teste) → SRS
  local em Docker.

## 11. Fora de escopo (v1 — YAGNI)

- Gravação/armazenamento de vídeo.
- Multi-máquina / painel central (cada instância gerencia a própria máquina).
- Autenticação avançada / multiusuário.
- Métricas históricas / banco de dados (só status em memória por enquanto).
- Saídas além de RTMP (HLS, WebRTC, etc.).

## 12. Nome / identidade

Projeto: **Atines Smart Stream**. Diretório: `atines_smart_stream`.
