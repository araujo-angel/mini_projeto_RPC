package remotelist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

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

// Persistência
type LogEntry struct {
	LSN       uint64 `json:"lsn"` //Log Sequence Number - "contador global"
	Timestamp int64  `json:"timestamp"`
	Operation string `json:"operation"` // "APPEND" ou "REMOVE"
	ListName  string `json:"list_name"`
	Value     int    `json:"value"` // 0 para REMOVE
}

type SnapshotData struct {
	LSN       uint64           `json:"lsn"`
	Timestamp int64            `json:"timestamp"`
	Lists     map[string][]int `json:"lists"` // Nome: dados
}

type RemoteList struct {
	mu         sync.RWMutex
	nameToUUID map[string]uuid.UUID
	lists      map[uuid.UUID][]int
	currentLSN uint64
	walFile    *os.File
}

// writeWAL escreve uma entrada no Write-Ahead Log
func (l *RemoteList) writeWAL(operation, listName string, value int) error {
	l.currentLSN++

	entry := LogEntry{
		LSN:       l.currentLSN,
		Timestamp: time.Now().Unix(),
		Operation: operation,
		ListName:  listName,
		Value:     value,
	}

	// Serializa para JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = l.walFile.WriteString(string(data) + "\n")
	if err != nil {
		return err
	}

	// fsync para garantir que está no disco
	err = l.walFile.Sync()
	if err != nil {
		return err
	}

	return nil
}

func (l *RemoteList) createSnapshot() error {
	l.mu.RLock()

	snapshotLSN := l.currentLSN

	uuidToName := make(map[uuid.UUID]string)
	for name, uid := range l.nameToUUID {
		uuidToName[uid] = name
	}

	listsData := make(map[string][]int)
	for uid, data := range l.lists {
		name := uuidToName[uid]
		listCopy := make([]int, len(data))
		copy(listCopy, data)
		listsData[name] = listCopy
	}

	l.mu.RUnlock()

	snapshot := SnapshotData{
		LSN:       snapshotLSN,
		Timestamp: time.Now().Unix(),
		Lists:     listsData,
	}

	os.MkdirAll("data", 0755)

	timestamp := time.Now().Unix()
	snapshotName := fmt.Sprintf("data/snapshot_%d.json", timestamp)
	tmpFile := snapshotName + ".tmp"

	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(snapshot)
	if err != nil {
		return err
	}

	err = file.Sync()
	if err != nil {
		return err
	}
	file.Close()

	err = os.Rename(tmpFile, snapshotName)
	if err != nil {
		return err
	}

	fmt.Printf("Snapshot criado: %s LSN=%d, %d listas\n", snapshotName, snapshotLSN, len(listsData))

	err = l.cleanOldSnapshots(3)
	if err != nil {
		fmt.Printf("Aviso: Erro ao limpar snapshots antigos: %v\n", err)
	}

	err = l.truncateWAL()
	if err != nil {
		fmt.Printf("Aviso: Erro ao truncar WAL: %v\n", err)
	}

	return nil
}

func (l *RemoteList) truncateWAL() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.walFile != nil {
		l.walFile.Close()
	}

	walFile, err := os.OpenFile("data/wal.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("erro ao truncar WAL: %v", err)
	}

	l.walFile = walFile
	fmt.Printf("WAL truncado (operacoes ate LSN=%d ja estao no snapshot)\n", l.currentLSN)
	return nil
}

func (l *RemoteList) cleanOldSnapshots(keepCount int) error {
	files, err := os.ReadDir("data")
	if err != nil {
		return err
	}

	var snapshots []string
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "snapshot_") && strings.HasSuffix(name, ".json") {
			snapshots = append(snapshots, name)
		}
	}

	if len(snapshots) <= keepCount {
		return nil
	}

	sort.Strings(snapshots)

	toRemove := snapshots[:len(snapshots)-keepCount]
	for _, oldSnapshot := range toRemove {
		fullPath := filepath.Join("data", oldSnapshot)
		err := os.Remove(fullPath)
		if err != nil {
			fmt.Printf("Aviso: Erro ao remover snapshot antigo %s: %v\n", oldSnapshot, err)
		} else {
			fmt.Printf("Snapshot antigo removido: %s\n", oldSnapshot)
		}
	}

	return nil
}

func (l *RemoteList) findLatestSnapshot() (string, error) {
	files, err := os.ReadDir("data")
	if err != nil {
		return "", err
	}

	var snapshots []string
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "snapshot_") && strings.HasSuffix(name, ".json") {
			snapshots = append(snapshots, name)
		}
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("nenhum snapshot encontrado")
	}

	sort.Strings(snapshots)
	latestSnapshot := snapshots[len(snapshots)-1]

	return filepath.Join("data", latestSnapshot), nil
}

func (l *RemoteList) startSnapshotRoutine(intervalSeconds int) {
	go func() {
		ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			err := l.createSnapshot()
			if err != nil {
				fmt.Printf("Erro ao criar snapshot: %v\n", err)
			}
		}
	}()
	fmt.Printf("Snapshot automático iniciado (intervalo: %ds)\n", intervalSeconds)
}

func (l *RemoteList) Recover() error {
	fmt.Println("\n=== Iniciando Recuperação ===")

	var snapshotLSN uint64 = 0

	snapshotFile, err := l.findLatestSnapshot()
	if err != nil {
		fmt.Println(" Nenhum snapshot encontrado, iniciando do zero")
	} else {
		fmt.Printf("Snapshot encontrado: %s\n", snapshotFile)

		file, err := os.Open(snapshotFile)
		if err != nil {
			return fmt.Errorf("erro ao abrir snapshot: %v", err)
		}
		defer file.Close()

		var snapshot SnapshotData
		decoder := json.NewDecoder(file)
		err = decoder.Decode(&snapshot)
		if err != nil {
			return fmt.Errorf("erro ao decodificar snapshot: %v", err)
		}

		snapshotLSN = snapshot.LSN
		l.currentLSN = snapshotLSN

		for listName, data := range snapshot.Lists {
			listUUID := uuid.New()
			l.nameToUUID[listName] = listUUID
			l.lists[listUUID] = data
		}

		fmt.Printf(" LSN do snapshot: %d\n", snapshotLSN)
		fmt.Printf(" Listas restauradas: %d\n", len(snapshot.Lists))
	}

	//Replay do WAL
	walFile := "data/wal.log"
	if _, err := os.Stat(walFile); err == nil {
		fmt.Println(" WAL encontrado, aplicando operações...")

		file, err := os.Open(walFile)
		if err != nil {
			return fmt.Errorf("erro ao abrir WAL: %v", err)
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		appliedOps := 0

		for {
			var entry LogEntry
			err := decoder.Decode(&entry)
			if err != nil {
				break
			}

			// Aplica apenas operações APÓS o snapshot
			if entry.LSN <= snapshotLSN {
				continue
			}

			// Replay da operação
			switch entry.Operation {
			case "APPEND":
				listUUID := l.getOrCreateListUUID(entry.ListName)
				l.lists[listUUID] = append(l.lists[listUUID], entry.Value)
			case "REMOVE":
				if listUUID, exists := l.nameToUUID[entry.ListName]; exists {
					if len(l.lists[listUUID]) > 0 {
						l.lists[listUUID] = l.lists[listUUID][:len(l.lists[listUUID])-1]
					}
				}
			}

			l.currentLSN = entry.LSN
			appliedOps++
		}

		fmt.Printf("  Operações aplicadas do WAL: %d\n", appliedOps)
		fmt.Printf("  LSN final: %d\n", l.currentLSN)
	} else {
		fmt.Println(" Nenhum WAL encontrado")
	}

	fmt.Printf("\n Recuperação completa! Estado restaurado.\n")
	fmt.Printf("  Total de listas: %d\n", len(l.nameToUUID))
	fmt.Println("================================\n")

	return nil
}

// Retorna o UUID de uma lista, criando-a se não existir
func (l *RemoteList) getOrCreateListUUID(name string) uuid.UUID {
	if listUUID, exists := l.nameToUUID[name]; exists {
		return listUUID
	}
	newUUID := uuid.New()
	l.nameToUUID[name] = newUUID
	l.lists[newUUID] = make([]int, 0)
	fmt.Printf("Nova lista criada: '%s' UUID: %s\n", name, newUUID)
	return newUUID
}

func (l *RemoteList) Append(args AppendArgs, reply *bool) error {
	l.mu.Lock() // Write lock - acesso exclusivo (bloqueia leitores e escritores)
	defer l.mu.Unlock()

	err := l.writeWAL("APPEND", args.ListName, args.Value)
	if err != nil {
		return fmt.Errorf("erro ao escrever WAL: %v", err)
	}

	listUUID := l.getOrCreateListUUID(args.ListName)
	l.lists[listUUID] = append(l.lists[listUUID], args.Value)
	fmt.Printf("Lista '%s': %v\n", args.ListName, l.lists[listUUID])

	*reply = true
	return nil
}

func (l *RemoteList) Get(args GetArgs, reply *int) error {
	l.mu.RLock() // Read lock - permite múltiplos leitores
	defer l.mu.RUnlock()

	*reply = 0

	listUUID, exists := l.nameToUUID[args.ListName]
	if !exists {
		return errors.New("list not found")
	}

	list := l.lists[listUUID]
	if args.Index < 0 || args.Index >= len(list) {
		return errors.New("index out of bounds")
	}

	*reply = list[args.Index]
	return nil
}

func (l *RemoteList) Remove(args RemoveArgs, reply *int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	*reply = 0

	listUUID, exists := l.nameToUUID[args.ListName]
	if !exists {
		return errors.New("list not found")
	}

	list := l.lists[listUUID]
	if len(list) == 0 {
		return errors.New("empty list")
	}

	// Captura valor antes de remover
	removedValue := list[len(list)-1]
	err := l.writeWAL("REMOVE", args.ListName, removedValue)
	if err != nil {
		return fmt.Errorf("erro ao escrever WAL: %v", err)
	}

	*reply = removedValue
	l.lists[listUUID] = list[:len(list)-1]
	fmt.Printf("Lista '%s': %v (removido: %d)\n", args.ListName, l.lists[listUUID], *reply)
	return nil
}

func (l *RemoteList) Size(args SizeArgs, reply *int) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	listUUID, exists := l.nameToUUID[args.ListName]
	if !exists {
		*reply = 0
		return nil
	}

	*reply = len(l.lists[listUUID])
	return nil
}

func (l *RemoteList) ListAll(args int, reply *ListAllReply) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	names := make([]string, 0, len(l.nameToUUID))
	for name := range l.nameToUUID {
		names = append(names, name)
	}

	reply.ListNames = names
	fmt.Printf("Listas existentes: %v\n", names)
	return nil
}

func NewRemoteList() *RemoteList {
	os.MkdirAll("data", 0755)

	walFile, err := os.OpenFile("data/wal.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(fmt.Sprintf("Erro ao abrir WAL: %v", err))
	}

	list := &RemoteList{
		nameToUUID: make(map[string]uuid.UUID),
		lists:      make(map[uuid.UUID][]int),
		currentLSN: 0,
		walFile:    walFile,
	}

	err = list.Recover()
	if err != nil {
		panic(fmt.Sprintf("Erro na recuperação: %v", err))
	}

	list.startSnapshotRoutine(120)

	return list
}
