# 🧬 HSEM v3.1: Especificação Técnica Mestre

## Arquitetura: Full Spectrum Bio-Emulation

**Versão:** 3.1 (Full Spectrum)
**Escopo:** Transporte, Sessão, Presença e Mídia
**Motor:** Whatsmeow (Go)
**Princípio Central:** "The Single Thumb Rule" (O dispositivo é mono-tarefa e o humano é mono-foco).

---

## 1. Definição do Problema e Solução

### 1.1. O Vetor de Banimento

O WhatsApp analisa não apenas *o que* é enviado, mas a **telemetria da sessão**. Comportamentos robóticos incluem:

1. **Concorrência:** Digitar para A e B simultaneamente.
2. **Onisciência:** Ler (Blue Tick) a mensagem no milissegundo exato da entrega.
3. **Presença Estática:** Ficar `Online` 24/7 sem nunca ir para `Offline/Background`.
4. **Incongruência de Mídia:** Enviar áudio "gravado" sem evento de `Recording`.

### 1.2. A Solução: Virtual Sub-Queues (VSQ) + Session Lifecycle

O HSEM v3.1 atua como um **Proxy de Coerência**. Ele gerencia o estado global da conexão (Sessão) e serializa ações dentro de contextos isolados (VSQ), garantindo que apenas *uma* interação humana ocorra por vez.

---

## 2. Ciclo de Vida da Sessão (Global State)

Antes de processar qualquer mensagem, o sistema deve gerenciar o estado do socket WebSocket.

### 2.1. Máquina de Estados de Conexão

O sistema opera em 4 estados macro:

1. **DORMANT (Background/Ghost):**
* *Socket:* Conectado.
* *Presence:* Nenhuma (ou `unavailable`).
* *Comportamento:* Recebe mensagens (push), mas não envia recibos de leitura nem presença. Acumula tarefas na fila.


2. **WARMING UP (Wake Lock):**
* *Gatilho:* Fila de saída > 0 ou Timer Aleatório de Verificação.
* *Ação:* Envia `Presence: Available`.
* *Delay:* `2s - 5s` (Simulação de abertura do app e carregamento de chats).


3. **ACTIVE FLOW (O Loop VSQ):**
* *Estado:* O usuário está ativamente trocando mensagens.
* *Lógica:* Executa o pipeline de filas (ver Seção 3).


4. **COOL DOWN (Exit Strategy):**
* *Gatilho:* Fila de saída vazia por `T > 30s`.
* *Ação:* Mantém `Available` por `Random(10s, 60s)` (Humano lendo conversas antigas ou distraído).
* *Transição:* Envia `Presence: Unavailable` -> Vai para DORMANT.



---

## 3. Arquitetura de Processamento (O Pipeline VSQ)

Quando em **ACTIVE FLOW**, o processamento segue blocos sequenciais estritos.

### Estágio 1: Ingestão e Buffering

Mensagens de saída entram no `Coherence Buffer` com janela `T_window` (300ms).

* *Objetivo:* Agrupar mensagens sequenciais para o mesmo destinatário.

### Estágio 2: Classificação (Binning)

O buffer organiza mensagens em filas virtuais por `JID` (Destinatário):

* **Contexto A:** `[Msg_A1, Msg_A2]`
* **Contexto B:** `[Msg_B1]`

### Estágio 3: Agendamento e Leitura (The Scheduler)

O sistema escolhe *um* contexto (Mutex). Antes de enviar, ele deve **processar a entrada**.

#### 3.1. A Física de Leitura (Receipts)

Se houver mensagens não lidas no Contexto A, o sistema deve executar o protocolo de leitura *antes* da resposta:

1. **Visual Interaction:** Envia `Presence: Available` (se já não estiver).
2. **Cognitive Delay ():**


3. **Action:** Envia `MarkRead` (Blue Tick).
4. **Reaction Gap:** Pausa de `0.5s` a `1.5s` (tempo de levar o dedo ao teclado).

### Estágio 4: Execução de Envio

Executa o plano de envio para o bloco.

---

## 4. Modelagem Matemática de Execução

### 4.1. Equação de Custo do Bloco ()

Onde:

* **:** Tempo de preparação (buscar sticker, abrir galeria, focar input).
* **:** Tempo físico de "transmissão" (digitação ou gravação).
* **:** Pausa entre mensagens do mesmo bloco.

### 4.2. Custo de Troca de Contexto ()

* *Nota:* Este tempo ocorre após o fim do Bloco A e antes do início da *Leitura* do Bloco B.

---

## 5. Protocolos de Mídia Estendidos (Whatsmeow Specs)

Cada tipo de mídia exige uma dança específica de estados de presença (`ChatPresence`).

### 5.1. Protocolo de Texto (Com Correção de Erro)

1. **Presence:** `Composing`.
2. **Wait (Typing):** `Len(char) * SpeedFactor`.
* *Simulação de Erro:* Adiciona 15% de tempo extra randomicamente (simulando backspace).


3. **Action:** Envio Payload (Texto).
4. **Presence:** `Paused`.

### 5.2. Protocolo de Áudio Gravado (PTT/Voice Note)

⚠️ **Risco Alto:** Metadados incorretos geram banimento.

1. **Presence:** `Recording`.
2. **Wait (Recording):** `Duration_File` + `Rand(0.5s, 1.0s)` (Margem de segurança).
3. **Action:** Envio Payload (Audio/Opus) + `PTT: true`.
4. **Presence:** `Paused`.

### 5.3. Protocolo de Áudio Encaminhado (Forwarded)

1. **Presence:** `Composing` (Breve) ou NENHUM. **Nunca** use `Recording`.
2. **Wait (UI):** `1.5s` (Selecionar mensagem -> Encaminhar).
3. **Action:** Envio Payload + `ContextInfo: { IsForwarded: true }`.

### 5.4. Protocolo de Imagem/Vídeo (Galeria vs. Câmera)

O sistema deve decidir a origem da mídia (`origin_type`).

* **Caso A: Câmera (Instantâneo)**
1. **Presence:** `Composing` -> `Recording` (alguns clientes mostram "Gravando vídeo").
2. **Wait:** Duração da mídia (Captura).
3. **Action:** Envio.


* **Caso B: Galeria (Arquivo)**
1. **Presence:** `Composing`.
2. **Wait (Search):** `2s - 5s` (Procurando na galeria).
3. **Wait (Upload):** `FileSize / BandwidthSim` (Simula barra de progresso).
4. **Action:** Envio.



### 5.5. Protocolo de Stickers (Figurinhas)

1. **Classification:**
* *Favorito (Hot Cache):* Acesso rápido.
* *Search (Cold Cache):* Acesso lento.


2. **Presence:** `Composing`.
3. **Wait:**
* *Fav:* `0.5s`.
* *Search:* `2.0s` a `4.0s`.


4. **Action:** Envio Payload (WebP).

### 5.6. Protocolo de Documentos/PDF

1. **Presence:** `Composing`.
2. **Wait:** `3.0s` (Busca no sistema de arquivos).
3. **Action:** Envio Payload + `FileName`.

---

## 6. Matriz de Personas (Modulação de Comportamento)

A Persona dita a velocidade e a probabilidade de erros.

| ID | Perfil | WPM (Texto) | Prob. Erro (Typos) | Comportamento de Leitura |
| --- | --- | --- | --- | --- |
| **`EXECUTIVE`** | Objetivo | 1.5x (Rápido) | 2% (Baixo) | Lê rápido, responde curto. Gap entre blocos é mínimo. |
| **`CASUAL`** | Relaxado | 1.0x (Médio) | 8% (Médio) | Demora para ler (distraído). Usa muitos Stickers. |
| **`BOOMER`** | Lento | 0.6x (Baixo) | 15% (Alto) | Digita devagar, apaga muito. Grava áudios longos. Delay de leitura alto. |
| **`GHOST`** | Evasivo | Var. Alta | 0% | Desativa confirmação de leitura (opcional na config). Responde em rajadas. |

---

## 7. Configuração JSON (Schema v3.1)

```json
{
  "hsem_core": {
    "version": "3.1",
    "mode": "full_spectrum"
  },
  "session_lifecycle": {
    "keep_alive_after_last_msg_sec": [10, 45],
    "poll_interval_ms": 500
  },
  "read_receipts": {
    "enabled": true,
    "strategy": "cognitive_delay",
    "wpm_reading": 200
  },
  "media_physics": {
    "ptt_padding_ms": 800,
    "image_gallery_search_ms": [2000, 5000],
    "sticker_selection_ms": {
      "favorite": 500,
      "search": 3500
    }
  },
  "personas": [
    {
      "id": "standard_human",
      "wpm_typing": [50, 70],
      "error_correction_rate": 0.05,
      "preferred_media": "mixed"
    }
  ]
}

```
