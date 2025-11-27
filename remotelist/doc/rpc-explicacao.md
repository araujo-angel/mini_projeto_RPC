# RPC: Conceitos e Implementação em Go

Este documento explica os conceitos fundamentais de RPC (Remote Procedure Call) e como eles são implementados neste projeto utilizando a biblioteca nativa `net/rpc` do Go.

## 1. Identificando um Sistema RPC

Um sistema baseado em RPC possui as seguintes características:

### Evidências no Código

| Localização | Código | Significado |
|-------------|--------|-------------|
| `server.go:7` | `import "net/rpc"` | Importação da biblioteca RPC do Go |
| `server.go:12` | `rpcs := rpc.NewServer()` | Criação de um servidor RPC |
| `server.go:13` | `rpcs.Register(list)` | Registro de objeto para chamadas remotas |
| `client.go:6` | `import "net/rpc"` | Importação da biblioteca RPC no cliente |
| `client.go:12` | `client, err := rpc.Dial("tcp", ":5000")` | Estabelecimento de conexão RPC |
| `client.go:22` | `client.Call("RemoteList.Append", args, &reply)` | Execução de chamada de procedimento remoto |

**Classificação:** Este projeto implementa RPC utilizando a biblioteca nativa do Go (`net/rpc`) com serialização no formato Gob.

---

## 2. Arquitetura do Sistema RPC

O sistema segue a arquitetura clássica de RPC com separação clara entre cliente, servidor e interface compartilhada:

```
┌─────────────────────┐         REDE TCP          ┌──────────────────────┐
│   CLIENTE           │         :5000             │   SERVIDOR           │
│  (client.go)        │<───────────────────────>│  (server.go)         │
└─────────────────────┘                           └──────────────────────┘
         │                                                   │
         │                                                   │
    CLIENT STUB                                        SERVER STUB
   (rpc.Client)                                       (rpc.Server)
         │                                                   │
         │                                                   │
         └───────────── Structs compartilhados ─────────────┘
                      (remotelist_rpc.go)
```

### 2.1. RPC e Arquitetura Cliente-Servidor

Este projeto implementa uma **arquitetura cliente-servidor simples** onde:
- **Um único servidor** centraliza o estado (listas remotas) e processa requisições
- **Múltiplos clientes** conectam-se ao servidor e executam operações remotas
- A comunicação é **síncrona**: o cliente aguarda a resposta antes de continuar

O RPC é utilizado como mecanismo de comunicação entre cliente e servidor, abstraindo a complexidade de rede e serialização. O cliente invoca métodos remotos (`Append`, `Get`, `Remove`) como se fossem locais, enquanto o RPC cuida da transferência de dados.

**Outras arquiteturas com RPC:**

RPC não se limita a sistemas cliente-servidor simples. Pode ser aplicado em:
- **Sistemas distribuídos peer-to-peer:** Nós se comunicam diretamente sem servidor central
- **Microserviços:** Serviços independentes comunicam-se via RPC (ex: gRPC)
- **Arquiteturas em camadas:** Camadas de aplicação chamam procedimentos em camadas inferiores remotamente
- **Computação distribuída:** Processamento distribuído entre múltiplos nós

O conceito central permanece: permitir que programas executem procedimentos em espaços de endereçamento diferentes, independentemente da topologia da arquitetura.

---

## 3. Client Stub - Proxy de Comunicação

### Localização no Código

O Client Stub é criado em `client.go:12`:

```
client, err := rpc.Dial("tcp", ":5000")
```

### Responsabilidades

O Client Stub atua como um proxy que abstrai toda a complexidade da comunicação remota. Ele é responsável por:

1. Estabelecer e manter a conexão TCP com o servidor
2. Serializar (marshal) os argumentos das chamadas
3. Enviar dados pela rede
4. Aguardar a resposta do servidor
5. Desserializar (unmarshal) a resposta recebida
6. Retornar o resultado ou erro ao código do cliente

### Exemplo de Uso

```
client.Call("RemoteList.Append",
    remotelist.AppendArgs{ListName: "compras", Value: 10},
    &reply)
```

### Fluxo Interno do Client Stub

Quando uma chamada remota é executada, o Client Stub realiza as seguintes operações:

```
┌──────────────────────────────────────────────────────────────┐
│ CLIENT STUB (rpc.Client)                                     │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│ 1. PREPARAÇÃO                                                │
│    - Método: "RemoteList.Append"                            │
│    - Args: AppendArgs{ListName:"compras", Value:10}         │
│                                                              │
│ 2. MARSHALING (Serialização)                                │
│    ┌─────────────────────────────────────┐                 │
│    │ AppendArgs struct                   │                 │
│    │   ListName: "compras"               │                 │
│    │   Value: 10                         │                 │
│    └──────────────┬──────────────────────┘                 │
│                   │ gob.Encode()                            │
│                   ▼                                         │
│    ┌─────────────────────────────────────┐                 │
│    │ [bytes]: 0x1F 0x8B 0x08 0x00...    │                 │
│    └─────────────────────────────────────┘                 │
│                                                              │
│ 3. ENVIO PELA REDE                                          │
│    - Protocolo: TCP                                         │
│    - Porta: 5000                                            │
│    - Dados: bytes serializados                              │
│                                                              │
│ 4. AGUARDA RESPOSTA                                         │
│    - Chamada bloqueante até receber dados                   │
│                                                              │
│ 5. UNMARSHALING (Desserialização)                           │
│    ┌─────────────────────────────────────┐                 │
│    │ [bytes]: 0x01                       │                 │
│    └──────────────┬──────────────────────┘                 │
│                   │ gob.Decode()                            │
│                   ▼                                         │
│    ┌─────────────────────────────────────┐                 │
│    │ reply = true                        │                 │
│    └─────────────────────────────────────┘                 │
│                                                              │
│ 6. RETORNA AO CÓDIGO DO USUÁRIO                             │
│    - reply agora contém o valor retornado                   │
│    - err = nil (em caso de sucesso)                         │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 4. Server Stub - Dispatcher de Chamadas

### Localização no Código

O Server Stub é criado e configurado em `server.go:12-13`:

```
rpcs := rpc.NewServer()  // Criação do Server Stub
rpcs.Register(list)      // Registro da implementação real
```

### Responsabilidades

O Server Stub atua como um dispatcher que recebe chamadas remotas e as encaminha para a implementação real. Ele é responsável por:

1. Escutar conexões TCP na porta especificada
2. Receber dados serializados da rede
3. Desserializar (unmarshal) os argumentos recebidos
4. Localizar e invocar o método correspondente no objeto registrado
5. Capturar o resultado da execução
6. Serializar (marshal) a resposta
7. Enviar a resposta de volta ao cliente

### Processamento de Conexões

```
go rpcs.ServeConn(conn)  // Processa cada cliente em uma goroutine
```

### Fluxo Interno do Server Stub

Quando uma chamada remota é recebida, o Server Stub realiza as seguintes operações:

```
┌──────────────────────────────────────────────────────────────┐
│ SERVER STUB (rpc.Server)                                     │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│ 1. RECEBE DADOS DA REDE                                      │
│    - Lê bytes do socket TCP                                 │
│    - Dados: [bytes]: 0x1F 0x8B 0x08 0x00...                │
│                                                              │
│ 2. UNMARSHALING (Desserialização)                           │
│    ┌─────────────────────────────────────┐                 │
│    │ [bytes]: 0x1F 0x8B 0x08 0x00...    │                 │
│    └──────────────┬──────────────────────┘                 │
│                   │ gob.Decode()                            │
│                   ▼                                         │
│    ┌─────────────────────────────────────┐                 │
│    │ Método: "RemoteList.Append"         │                 │
│    │ Args: AppendArgs{                   │                 │
│    │   ListName: "compras"               │                 │
│    │   Value: 10                         │                 │
│    │ }                                   │                 │
│    └─────────────────────────────────────┘                 │
│                                                              │
│ 3. LOCALIZA O MÉTODO                                        │
│    - Busca objeto "RemoteList" (registrado)                 │
│    - Localiza método "Append" no objeto                     │
│                                                              │
│ 4. CHAMA IMPLEMENTAÇÃO REAL                                 │
│    ┌─────────────────────────────────────┐                 │
│    │ list.Append(                        │                 │
│    │   AppendArgs{                       │                 │
│    │     ListName: "compras",            │                 │
│    │     Value: 10                       │                 │
│    │   },                                │                 │
│    │   &reply                            │                 │
│    │ )                                   │                 │
│    └──────────────┬──────────────────────┘                 │
│                   │                                         │
│                   ▼                                         │
│         [Executa em remotelist_rpc.go:361-376]             │
│         - Grava operação no WAL                             │
│         - Adiciona valor à lista em memória                 │
│         - Define reply = true                               │
│                                                              │
│ 5. MARSHALING DA RESPOSTA                                   │
│    ┌─────────────────────────────────────┐                 │
│    │ reply = true                        │                 │
│    └──────────────┬──────────────────────┘                 │
│                   │ gob.Encode()                            │
│                   ▼                                         │
│    ┌─────────────────────────────────────┐                 │
│    │ [bytes]: 0x01                       │                 │
│    └─────────────────────────────────────┘                 │
│                                                              │
│ 6. ENVIA RESPOSTA                                           │
│    - Escreve bytes no socket TCP                            │
│    - Cliente recebe e processa                              │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 5. Mapeamento Completo dos Componentes

### Cliente (client.go)

**Criação do Client Stub:**
```
// Linha 12: Estabelece conexão e cria proxy
client, err := rpc.Dial("tcp", ":5000")
```

**Uso do Client Stub:**
```
// Linha 22: Executa chamada remota
client.Call("RemoteList.Append",           // Método a ser executado remotamente
    remotelist.AppendArgs{                 // Argumentos (serão serializados)
        ListName: "compras",
        Value: 10
    },
    &reply)                                // Variável para receber resultado
```

**Funções do Client Stub nesta implementação:**
- Serializa `AppendArgs` para formato binário (Gob)
- Transmite dados via protocolo TCP
- Aguarda resposta do servidor (operação bloqueante)
- Desserializa resposta recebida em `reply`
- Propaga erros de rede ou execução

---

### Servidor (server.go)

**Criação do objeto de implementação:**
```
// Linha 11: Instancia objeto com lógica de negócio
list := remotelist.NewRemoteList()
```

**Criação do Server Stub:**
```
// Linha 12: Cria servidor RPC
rpcs := rpc.NewServer()

// Linha 13: Registra implementação para receber chamadas
rpcs.Register(list)
```

**Configuração da rede:**
```
// Linha 14: Escuta conexões TCP na porta 5000
l, e := net.Listen("tcp", "[localhost]:5000")

// Linha 22: Processa cada conexão em goroutine separada
go rpcs.ServeConn(conn)
```

**Funções do Server Stub nesta implementação:**
- Recebe bytes da conexão TCP
- Desserializa dados para `AppendArgs`
- Invoca `list.Append(args, &reply)` na implementação real
- Serializa `reply` para formato binário (Gob)
- Transmite resposta de volta ao cliente

---

### Implementação Real (remotelist_rpc.go)

**Método executado pelo Server Stub:**
```go
// Linha 361-376: Lógica de negócio real
func (l *RemoteList) Append(args AppendArgs, reply *bool) error {
    l.mu.Lock()
    defer l.mu.Unlock()

    // Grava operação no Write-Ahead Log
    err := l.writeWAL("APPEND", args.ListName, args.Value)
    if err != nil {
        return fmt.Errorf("erro ao escrever WAL: %v", err)
    }

    // Modifica estrutura de dados em memória
    listUUID := l.getOrCreateListUUID(args.ListName)
    l.lists[listUUID] = append(l.lists[listUUID], args.Value)

    *reply = true
    return nil
}
```

Este método não tem conhecimento de que está sendo chamado remotamente. Do ponto de vista da implementação, é apenas uma função local.

---

## 6. Fluxo Completo de uma Chamada RPC

O diagrama abaixo mostra o fluxo completo de uma operação `Append` desde o cliente até o servidor e a resposta de volta:

```
CLIENTE                 CLIENT STUB           REDE           SERVER STUB              SERVIDOR
========                ===========           ====           ===========              ========

[client.go:22]
client.Call(
  "RemoteList.Append"
  AppendArgs{...}
  &reply
)
    │
    │
    ▼
┌─────────────┐
│ CLIENT STUB │
│  (rpc.Dial) │
└──────┬──────┘
       │
       │ 1. Marshal
       │    AppendArgs
       │    → bytes
       │
       │ 2. Send TCP
       ▼
    [bytes]  ───────────────>  [bytes]
                                │
                                │
                                ▼
                          ┌──────────────┐
                          │ SERVER STUB  │
                          │(rpc.Server)  │
                          └──────┬───────┘
                                 │
                                 │ 3. Unmarshal
                                 │    bytes → args
                                 │
                                 │ 4. Lookup
                                 │    "RemoteList.Append"
                                 │
                                 │ 5. Call
                                 ▼
                          [remotelist_rpc.go:361]
                          func (l *RemoteList) Append(
                              args AppendArgs,
                              reply *bool
                          ) error {
                              // WAL
                              // Modifica lista
                              *reply = true  <───┐
                              return nil         │
                          }                      │
                                 │               │
                                 │ 6. Return     │
                                 ▼               │
                          ┌──────────────┐       │
                          │ SERVER STUB  │       │
                          └──────┬───────┘       │
                                 │               │
                                 │ 7. Marshal    │
                                 │    reply → bytes
                                 │               │
                                 │ 8. Send TCP   │
                                 ▼               │
                              [bytes] <──────────┘
                                 │
    [bytes] <────────────────────│
       │                         │
       ▼                         │
┌─────────────┐                  │
│ CLIENT STUB │                  │
└──────┬──────┘                  │
       │                         │
       │ 9. Unmarshal            │
       │    bytes → reply        │
       │                         │
       │ 10. Return              │
       ▼                         │
reply = true <──────────────────┘

[cliente continua...]
```

---

## 7. Estrutura de Dados Compartilhada

A interface entre cliente e servidor é definida através de structs compartilhadas em `remotelist_rpc.go`:

```go
type AppendArgs struct {
    ListName string
    Value    int
}

type GetArgs struct {
    ListName string
    Index    int
}

type RemoveArgs struct {
    ListName string
}

type SizeArgs struct {
    ListName string
}

type ListAllReply struct {
    ListNames []string
}
```

Estas estruturas funcionam como o contrato de comunicação entre cliente e servidor, definindo:
- Quais parâmetros cada operação remota aceita
- Qual o formato das respostas esperadas
- O tipo de dados que será serializado e transmitido pela rede

---

## 8. Serialização: Gob vs JSON

O projeto utiliza dois tipos de serialização para propósitos diferentes:

### Serialização Gob (RPC - Comunicação de Rede)

A biblioteca `net/rpc` do Go utiliza automaticamente o formato **Gob** para serializar dados transmitidos pela rede. Características:

- **Formato binário** (mais eficiente que texto)
- **Específico do Go** (não interoperável com outras linguagens)
- **Automático** (feito pela biblioteca RPC)
- **Propósito:** Comunicação cliente-servidor

### Serialização JSON (WAL - Persistência)

O Write-Ahead Log utiliza **JSON** para gravar operações no disco (`remotelist_rpc.go:75`):

```
data, err := json.Marshal(entry)
```

Características:

- **Formato texto** (legível por humanos)
- **Interoperável** (pode ser lido por outras ferramentas)
- **Explícito** (você controla quando serializar)
- **Propósito:** Persistência e recuperação de dados

**Relação conceitual:** Ambos são marshaling (serialização), mas aplicados em contextos diferentes:
- **Gob:** Transformar `struct` → `bytes` para rede
- **JSON:** Transformar `struct` → texto para disco

---

## 9. Resumo dos Componentes

| Componente | Localização no Código | Função |
|------------|----------------------|---------|
| **Client Stub** | `client.go:12` - `rpc.Dial()` | Proxy que abstrai comunicação remota |
| **Server Stub** | `server.go:12-13` - `rpc.NewServer()` + `Register()` | Dispatcher que roteia chamadas para implementação |
| **Implementação** | `remotelist_rpc.go:361-476` | Lógica de negócio (WAL, manipulação de listas) |
| **Contratos** | `remotelist_rpc.go:17-36` | Structs que definem interface de comunicação |

---

## 10. Conceitos Teóricos de RPC

Os conceitos fundamentais de RPC são independentes da linguagem:

1. **Transparência de Localização:** O cliente chama métodos remotos como se fossem locais
2. **Client Stub:** Componente que encapsula a comunicação do lado do cliente
3. **Server Stub:** Componente que despacha chamadas do lado do servidor
4. **Marshaling/Unmarshaling:** Serialização e desserialização de parâmetros
5. **Protocolo de Transporte:** Camada de rede para transmissão (TCP neste caso)
6. **Interface Definida:** Contrato compartilhado entre cliente e servidor

A implementação em Go utiliza a biblioteca `net/rpc` que fornece Client Stub e Server Stub prontos, enquanto o desenvolvedor precisa apenas:
- Definir as estruturas de dados (contratos)
- Implementar os métodos com a assinatura correta
- Configurar a conexão de rede