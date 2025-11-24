# RemoteList RPC - Sistema de Listas Distribuídas

Sistema cliente-servidor que implementa listas remotas compartilhadas usando RPC (Remote Procedure Call) em Go, com persistência durável e acesso concorrente.

## Características Principais

- **RPC sobre TCP**: Comunicação cliente-servidor usando `net/rpc` do Go
- **Múltiplos Clientes**: Suporte a acesso concorrente com exclusão mútua
- **Persistência Híbrida**: WAL (Write-Ahead Log) + Snapshots periódicos
- **Recuperação Automática**: Restaura estado após falhas
- **Thread-Safe**: Sincronização com `sync.RWMutex`

## Operações Disponíveis

| Operação | Descrição | Tipo |
|----------|-----------|------|
| `Append(list_name, value)` | Adiciona valor ao final da lista | Escrita |
| `Get(list_name, index)` | Retorna valor em posição específica | Leitura |
| `Remove(list_name)` | Remove e retorna último elemento | Escrita |
| `Size(list_name)` | Retorna tamanho da lista | Leitura |
| `ListAll()` | Lista todas as listas existentes | Leitura |

## Arquitetura do Sistema

O sistema utiliza:
- **Mapeamento Híbrido**: Nomes (strings) → UUIDs internos
- **Memória Principal**: `map[uuid.UUID][]int` para dados
- **Persistência em Disco**: WAL + Snapshots a cada 120s
- **Locks**: RWMutex permite leituras paralelas e escritas exclusivas

## Diagramas de Sequência

### Operação GET (Leitura)

![Diagrama de Sequência - GET](./remotelist/doc/get-sequence.png)

Operações de leitura (`Get`, `Size`, `ListAll`):
- Usam `RLock` (Read Lock)
- Permitem múltiplas leituras simultâneas
- Não bloqueiam outras leituras, apenas escritas
- Retornam dados diretamente da memória

### Operação REMOVE (Escrita)

![Diagrama de Sequência - REMOVE](./remotelist/doc/remove-sequence.png)

Operações de escrita (`Append`, `Remove`):
1. Adquire `Lock` exclusivo
2. Grava operação no WAL (com `fsync`)
3. Atualiza estado em memória
4. Libera lock
5. Retorna resposta ao cliente

### Snapshot Background Task

![Diagrama de Sequência - Background](./remotelist/doc/background-sequence.png)

Processo automático a cada 120 segundos:
1. Usa `RLock` para copiar dados da memória
2. Cria arquivo snapshot temporário (.tmp)
3. Grava JSON e sincroniza com disco
4. Renomeia para arquivo final (operação atômica)
5. Remove snapshots antigos (mantém 3)
6. Trunca o WAL (operações já estão no snapshot)

### Modelo de Concorrência

![Diagrama de Concorrência](./remotelist/doc/concorrencia-sequence.png)

**Estratégia de Locks:**
- **Write Lock** (`mu.Lock`): Bloqueia todas as operações (leitura e escrita)
- **Read Lock** (`mu.RLock`): Permite múltiplas leituras simultâneas, bloqueia apenas escritas

**Goroutines:**
- **Main**: Servidor RPC + handlers de requisições (uma goroutine por cliente)
- **Background**: Timer de 120s que cria snapshots automáticos

## Persistência e Recuperação

### Write-Ahead Log (WAL)
```
{"lsn":1,"timestamp":1699564800,"operation":"APPEND","list_name":"compras","value":10}
{"lsn":2,"timestamp":1699564801,"operation":"REMOVE","list_name":"compras","value":10}
```
*Formato: JSON Lines (JSONL) - cada linha é um objeto JSON independente*

### Snapshot
```json
{
  "lsn": 50,
  "timestamp": 1699565000,
  "lists": {
    "compras": [10, 20, 30],
    "tarefas": [100, 200]
  }
}
```

### Limpeza Automatica de Arquivos

O sistema implementa gerenciamento automatico de arquivos de persistencia para controlar uso de disco:

#### Truncamento de WAL
- **Quando**: Apos cada snapshot bem-sucedido (a cada 120 segundos)
- **Como**: O arquivo WAL e truncado (resetado para vazio)
- **Seguranca**: Todas as operacoes ate o LSN do snapshot ja estao persistidas
- **Tamanho maximo**: Com snapshot a cada 120s, o WAL nunca cresce alem de ~120s de operacoes

#### Rotacao de Snapshots
- **Formato de nome**: `snapshot_<timestamp>.json` (ex: `snapshot_1699564800.json`)
- **Retencao**: Mantem apenas os 3 snapshots mais recentes
- **Limpeza**: Automatica apos cada novo snapshot
- **Espaco**: Limitado a ~3x o tamanho de um snapshot
- **Recovery**: Sempre usa o snapshot mais recente disponivel

**Exemplo de ciclo completo:**
```
1. Snapshot criado: snapshot_1699564800.json (LSN=100)
2. WAL truncado (agora vazio)
3. Novas operacoes LSN=101, 102, 103... gravadas no WAL
4. 120s depois: snapshot_1699564920.json criado (LSN=150)
5. WAL truncado novamente
6. Processo se repete...
7. Ao atingir 4 snapshots, o mais antigo e removido automaticamente
```

**Estrutura de arquivos no diretorio data/:**
```
data/
├── wal.log                    (sempre pequeno, <1MB)
├── snapshot_1699564770.json   (penultimo)
├── snapshot_1699564800.json   (antepenultimo)
└── snapshot_1699564830.json   (mais recente, usado no recovery)
```

### Algoritmo de Recovery
```
1. Carregar snapshot.json (se existir) → estado base + LSN
2. Ler wal.log e aplicar apenas entradas com LSN > snapshot LSN
3. Sistema pronto com estado consistente
```

## Como Usar

### Pré-requisitos
- Go 1.21.5+

### Executar Servidor
```bash
cd remotelist
go run pkg_server/remotelist_rpc_server.go
```

### Executar Cliente
```bash
cd remotelist
go run pkg_client/remotelist_rpc_client.go
```

### Compilar
```bash
# Servidor
go build -o server.exe pkg_server/remotelist_rpc_server.go

# Cliente
go build -o client.exe pkg_client/remotelist_rpc_client.go
```

## Estrutura do Projeto

```
mini_projeto_RPC/
├── remotelist/
│   ├── pkg_structs/
│   │   └── remotelist_rpc.go       # Structs e lógica principal
│   ├── pkg_server/
│   │   └── remotelist_rpc_server.go # Servidor RPC
│   ├── pkg_client/
│   │   └── remotelist_rpc_client.go # Cliente com testes
│   ├── doc/                         # Diagramas de sequência
│   │   ├── get-sequence.png
│   │   ├── remove-sequence.png
│   │   ├── background-sequence.png
│   │   └── concorrencia-sequence.png
│   └── data/                        # Gerado em runtime
│       ├── wal.log
│       ├── snapshot_<timestamp>.json (3 arquivos mantidos)
│       └── snapshot_<timestamp>.json.tmp (temporario durante criacao)
```

## Garantias e Limitações

### Garantias
- **Durabilidade**: Operações confirmadas nunca são perdidas (WAL com fsync)
- **Consistência**: Leituras sempre retornam dados confirmados mais recentes
- **Isolamento**: Escritas são serializadas, leituras são isoladas

### Limitações
- **Single Server**: Ponto único de falha
- **In-Memory**: Limitado pela RAM disponível
- **Escrita Síncrona**: ~100-200 ops/s (limitado por fsync)
- **Sem Replicação**: Não há backup ativo em outro servidor
- **Historico Limitado**: Mantem apenas 3 snapshots (ultimos 360 segundos com intervalo de 120s)

### Pontos de Falha da Gestao Automatica de Arquivos

A implementacao de truncamento de WAL e rotacao de snapshots possui os seguintes cenarios de falha conhecidos:

#### 1. Falha Durante Truncamento
**Cenario:** Sistema falha exatamente durante a operacao de truncamento do WAL (apos snapshot, durante Close/Reopen do arquivo)

**Consequencia:**
- WAL pode ficar em estado inconsistente ou vazio
- Na recuperacao, se o snapshot foi criado com sucesso, nao ha perda de dados
- Se snapshot falhou mas WAL foi truncado, operacoes desde o ultimo snapshot bem-sucedido sao perdidas

**Probabilidade:** Baixa (janela de ~10-20ms a cada 120s)

**Mitigacao:** Sistema sempre cria snapshot antes de truncar WAL, garantindo que dados estao persistidos

#### 2. Falha no Snapshot com WAL Cheio
**Cenario:** Snapshots consecutivos falham (ex: disco cheio, permissoes) enquanto WAL continua crescendo

**Consequencia:**
- WAL nunca e truncado e continua crescendo indefinidamente
- Sistema pode ficar sem espaco em disco
- Recovery pode ficar lento (muitas operacoes no WAL)

**Probabilidade:** Media (depende de condicoes do sistema de arquivos)

**Mitigacao Atual:** Sistema registra aviso no log mas continua operando

**Mitigacao Sugerida:** Implementar monitoramento de tamanho de WAL e alertas

#### 3. Perda de Operacoes Entre Snapshots
**Cenario:** Sistema falha depois de snapshot bem-sucedido mas antes de algumas operacoes serem gravadas no novo WAL

**Consequencia:**
- Operacoes entre ultimo snapshot e falha podem ser perdidas se WAL estava sendo truncado
- Janela de vulnerabilidade de ~10-20ms

**Probabilidade:** Muito baixa (requer timing preciso)

**Mitigacao:** WAL e truncado APOS snapshot ser gravado e sincronizado no disco

#### 4. Corrupcao Durante Operacao Concorrente
**Cenario:** Operacao de escrita tenta gravar no WAL enquanto truncateWAL() esta fechando/reabrindo arquivo

**Consequencia:**
- Operacao de escrita pode falhar com erro de arquivo fechado
- Cliente recebe erro e pode retentar operacao

**Probabilidade:** Baixa (truncateWAL usa Lock exclusivo)

**Mitigacao:** truncateWAL() adquire lock exclusivo (mu.Lock) antes de fechar arquivo

#### 5. Crescimento de Snapshot Individual
**Cenario:** Cada snapshot continua crescendo conforme dados aumentam (rotacao nao reduz tamanho individual)

**Consequencia:**
- Cada snapshot pode ficar muito grande (>100MB com muitos dados)
- Snapshot demorado bloqueia escritas brevemente durante copia de dados
- Uso de disco: 3 snapshots grandes = 3x tamanho dos dados

**Probabilidade:** Alta (em uso prolongado com muitos dados)

**Mitigacao Atual:** Snapshot usa RLock apenas para copiar dados, liberando antes de I/O; Rotacao limita quantidade total

**Mitigacao Sugerida:** Implementar compressao (gzip) nos snapshots

#### 6. Falha na Limpeza de Snapshots Antigos
**Cenario:** cleanOldSnapshots() falha (permissoes, disco cheio) mas novo snapshot foi criado

**Consequencia:**
- Snapshots antigos nao sao removidos
- Sistema acumula mais de 3 snapshots
- Uso de disco cresce gradualmente

**Probabilidade:** Baixa (depende de condicoes do sistema de arquivos)

**Mitigacao Atual:** Sistema registra aviso no log mas continua operando normalmente

**Mitigacao Sugerida:** Monitoramento de quantidade de arquivos no diretorio data/

#### 7. Recovery com Snapshot Corrompido
**Cenario:** Snapshot mais recente esta corrompido mas snapshots anteriores estao integros

**Consequencia:**
- Recovery falha ao ler snapshot mais recente
- Sistema nao tenta snapshots anteriores automaticamente
- Requer intervencao manual para remover snapshot corrompido

**Probabilidade:** Muito baixa (rename atomico protege contra corrupcao durante escrita)

**Mitigacao Atual:** Rename atomico garante que snapshot so e visivel quando completo

**Mitigacao Sugerida:** Implementar fallback automatico para snapshot anterior se o mais recente falhar

### Recomendacoes de Uso

Para minimizar riscos:
1. Monitorar tamanho de `data/wal.log` (deve ser pequeno, <1MB)
2. Monitorar quantidade de arquivos `snapshot_*.json` (deve ser exatamente 3 apos alguns ciclos)
3. Monitorar tamanho individual dos snapshots (indica crescimento de dados)
4. Monitorar logs do servidor para avisos de falha em snapshot/truncamento/limpeza
5. Fazer backup periodico do diretorio `data/` em ambiente de producao
6. Considerar aumentar intervalo de snapshot se houver muitas escritas (ex: 180s ou 300s ao inves de 120s)
7. Verificar permissoes do diretorio `data/` para garantir leitura/escrita/delecao

## Tecnologias

- **Linguagem**: Go 1.21.5
- **RPC**: `net/rpc` (biblioteca padrão)
- **Persistência**: JSON (WAL e Snapshots)
- **Sincronização**: `sync.RWMutex`
- **Identificação**: UUIDs v4 (`github.com/google/uuid`)

## Licença

Este projeto está sob a licença especificada no arquivo [LICENSE](LICENSE).