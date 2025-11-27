package main

import (
	"fmt"
	"ifpb/remotelist/pkg_structs"
	"net/rpc"
	"sync"
	"time"
)

func main() {
	client, err := rpc.Dial("tcp", ":5000")
	if err != nil {
		fmt.Print("dialing:", err)
		return
	}

	var reply bool
	var reply_i int

	fmt.Println("=== Lista 'compras' ===")
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "compras", Value: 10}, &reply)
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "compras", Value: 20}, &reply)
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "compras", Value: 30}, &reply)

	fmt.Println("\n=== Lista 'tarefas' ===")
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "tarefas", Value: 100}, &reply)
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "tarefas", Value: 200}, &reply)

	fmt.Println("\n=== Testando Size ===")
	err = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "compras"}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao obter tamanho:", err)
	} else {
		fmt.Printf("Lista 'compras': %d elementos\n", reply_i)
	}

	err = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "tarefas"}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao obter tamanho:", err)
	} else {
		fmt.Printf("Lista 'tarefas': %d elementos\n", reply_i)
	}

	fmt.Println("\n=== Testando Get ===")
	err = client.Call("RemoteList.Get", remotelist.GetArgs{ListName: "compras", Index: 1}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao obter elemento:", err)
	} else {
		fmt.Printf("Lista 'compras' - Posição 1: %d\n", reply_i)
	}

	err = client.Call("RemoteList.Get", remotelist.GetArgs{ListName: "tarefas", Index: 0}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao obter elemento:", err)
	} else {
		fmt.Printf("Lista 'tarefas' - Posição 0: %d\n", reply_i)
	}

	fmt.Println("\n=== Listando Todas as Listas (Discovery) ===")
	var listAll remotelist.ListAllReply
	err = client.Call("RemoteList.ListAll", 0, &listAll)
	if err != nil {
		fmt.Println("Erro ao listar:", err)
	} else {
		fmt.Printf("Listas disponíveis no servidor: %v\n", listAll.ListNames)
	}

	fmt.Println("\n=== Testando Remove ===")
	err = client.Call("RemoteList.Remove", remotelist.RemoveArgs{ListName: "compras"}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao remover:", err)
	} else {
		fmt.Printf("Lista 'compras' - Removido: %d\n", reply_i)
	}

	err = client.Call("RemoteList.Remove", remotelist.RemoveArgs{ListName: "tarefas"}, &reply_i)
	if err != nil {
		fmt.Println("Erro ao remover:", err)
	} else {
		fmt.Printf("Lista 'tarefas' - Removido: %d\n", reply_i)
	}

	fmt.Println("\n=== Tamanhos Finais ===")
	_ = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "compras"}, &reply_i)
	fmt.Printf("Lista 'compras': %d elementos\n", reply_i)

	_ = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "tarefas"}, &reply_i)
	fmt.Printf("Lista 'tarefas': %d elementos\n", reply_i)

	// Teste 1: Size de lista inexistente (retorna 0, sem erro)
	fmt.Println("\n[TESTE 1] Size de lista inexistente")
	fmt.Println("Lista: fantasma")
	fmt.Println("Esperado: SEM ERRO e reply = 0")
	err = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "fantasma"}, &reply_i)
	if err != nil {
		fmt.Printf("Resultado: ERRO '%v' | reply = %d\n", err, reply_i)
		fmt.Println("Status: FALHOU - nao deveria retornar erro")
	} else {
		fmt.Printf("Resultado: SEM ERRO | reply = %d\n", reply_i)
		if reply_i == 0 {
			fmt.Println("Status: PASSOU")
		} else {
			fmt.Println("Status: FALHOU - reply deveria ser 0")
		}
	}

	// Teste 2: Operacoes sequenciais (append e remove multiplos)
	fmt.Println("\n[TESTE 2] Operacoes sequenciais (3 appends + 2 removes)")
	fmt.Println("Lista: stress")
	fmt.Println("Esperado: Size final = 1")
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "stress", Value: 1}, &reply)
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "stress", Value: 2}, &reply)
	_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "stress", Value: 3}, &reply)
	_ = client.Call("RemoteList.Remove", remotelist.RemoveArgs{ListName: "stress"}, &reply_i)
	_ = client.Call("RemoteList.Remove", remotelist.RemoveArgs{ListName: "stress"}, &reply_i)
	_ = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "stress"}, &reply_i)
	fmt.Printf("Resultado: Size = %d\n", reply_i)
	if reply_i == 1 {
		fmt.Println("Status: PASSOU")
	} else {
		fmt.Println("Status: FALHOU")
	}

	// ====================================================================
	// TESTES DE CONCORRENCIA
	// ====================================================================
	fmt.Println("\n\n========================================")
	fmt.Println("TESTES DE CONCORRENCIA")
	fmt.Println("========================================")

	// Teste 3: Multiplas goroutines fazendo Append simultaneo
	fmt.Println("\n[TESTE 3] Multiplas goroutines fazendo Append simultaneo")
	fmt.Println("Configuracao: 10 goroutines, cada uma faz 10 appends")
	fmt.Println("Lista: concurrent_append")
	fmt.Println("Esperado: Size final = 100 (OBS: CASO O CLIENTE SEJA EXECUTADO MAIS DE UMA VEZ SEM APAGAR A PASTA DATA, O SIZE FINAL NAO SERÁ 100")

	var wg sync.WaitGroup
	numGoroutines := 10
	appendsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				fmt.Printf("Goroutine %d: erro ao conectar\n", id)
				return
			}
			defer localClient.Close()

			var localReply bool
			for j := 0; j < appendsPerGoroutine; j++ {
				_ = localClient.Call("RemoteList.Append", remotelist.AppendArgs{
					ListName: "concurrent_append",
					Value:    id*100 + j,
				}, &localReply)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // Pequeno delay para garantir que todas as operacoes foram processadas

	_ = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "concurrent_append"}, &reply_i)
	fmt.Printf("Resultado: Size = %d\n", reply_i)
	expectedSize := numGoroutines * appendsPerGoroutine
	if reply_i == expectedSize {
		fmt.Println("Status: PASSOU - Todas as operacoes concorrentes foram processadas corretamente")
	} else {
		fmt.Printf("Status: FALHOU - Esperado %d, recebido %d\n", expectedSize, reply_i)
	}

	// Teste 4: Leituras concorrentes (Get simultaneous)
	fmt.Println("\n[TESTE 4] Leituras concorrentes (Get simultaneo)")
	fmt.Println("Configuracao: Preparar lista 'read_test' com 5 elementos")
	fmt.Println("              20 goroutines fazem Get do mesmo indice simultaneamente")
	fmt.Println("Esperado: Todas as goroutines recebem o mesmo valor sem erro")

	// Preparacao
	for i := 0; i < 5; i++ {
		_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "read_test", Value: i * 10}, &reply)
	}

	successCount := 0
	errorCount := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				mu.Lock()
				errorCount++
				mu.Unlock()
				return
			}
			defer localClient.Close()

			var localReplyI int
			err = localClient.Call("RemoteList.Get", remotelist.GetArgs{ListName: "read_test", Index: 2}, &localReplyI)
			mu.Lock()
			if err == nil && localReplyI == 20 {
				successCount++
			} else {
				errorCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	fmt.Printf("Resultado: Leituras bem-sucedidas = %d | Erros = %d\n", successCount, errorCount)
	if successCount == 20 && errorCount == 0 {
		fmt.Println("Status: PASSOU - Todas as leituras concorrentes foram consistentes")
	} else {
		fmt.Println("Status: FALHOU - Houve inconsistencias nas leituras")
	}

	// Teste 5: Escritas e leituras concorrentes (mix de operacoes)
	fmt.Println("\n[TESTE 5] Escritas e leituras concorrentes (mix de operacoes)")
	fmt.Println("Configuracao: 5 goroutines fazendo Append, 5 fazendo Get")
	fmt.Println("Lista: mixed_ops")
	fmt.Println("Esperado: Nenhum erro de concorrencia, operacoes bem-sucedidas")

	// Preparacao inicial
	for i := 0; i < 3; i++ {
		_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "mixed_ops", Value: i}, &reply)
	}

	appendSuccess := 0
	getSuccess := 0
	totalErrors := 0

	// Goroutines fazendo Append
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				mu.Lock()
				totalErrors++
				mu.Unlock()
				return
			}
			defer localClient.Close()

			var localReply bool
			err = localClient.Call("RemoteList.Append", remotelist.AppendArgs{
				ListName: "mixed_ops",
				Value:    100 + id,
			}, &localReply)
			mu.Lock()
			if err == nil {
				appendSuccess++
			} else {
				totalErrors++
			}
			mu.Unlock()
		}(i)
	}

	// Goroutines fazendo Get
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond) // Pequeno delay para permitir alguns appends
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				mu.Lock()
				totalErrors++
				mu.Unlock()
				return
			}
			defer localClient.Close()

			var localReplyI int
			err = localClient.Call("RemoteList.Get", remotelist.GetArgs{ListName: "mixed_ops", Index: 0}, &localReplyI)
			mu.Lock()
			if err == nil {
				getSuccess++
			} else {
				totalErrors++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	fmt.Printf("Resultado: Appends bem-sucedidos = %d | Gets bem-sucedidos = %d | Erros = %d\n",
		appendSuccess, getSuccess, totalErrors)
	if appendSuccess == 5 && getSuccess == 5 && totalErrors == 0 {
		fmt.Println("Status: PASSOU - Operacoes mistas concorrentes funcionaram corretamente")
	} else {
		fmt.Println("Status: FALHOU - Houve problemas nas operacoes concorrentes")
	}

	// Teste 6: Append e Remove concorrentes na mesma lista
	fmt.Println("\n[TESTE 6] Append e Remove concorrentes na mesma lista")
	fmt.Println("Configuracao: 5 goroutines fazendo Append, 3 fazendo Remove")
	fmt.Println("Lista: append_remove_race")
	fmt.Println("Esperado: Nenhum panic, operacoes atomicas, size final consistente")

	// Preparacao inicial
	for i := 0; i < 5; i++ {
		_ = client.Call("RemoteList.Append", remotelist.AppendArgs{ListName: "append_remove_race", Value: i}, &reply)
	}

	appendOps := 0
	removeOps := 0
	removeErrors := 0

	// Goroutines fazendo Append
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				return
			}
			defer localClient.Close()

			var localReply bool
			for j := 0; j < 3; j++ {
				err = localClient.Call("RemoteList.Append", remotelist.AppendArgs{
					ListName: "append_remove_race",
					Value:    id*10 + j,
				}, &localReply)
				if err == nil {
					mu.Lock()
					appendOps++
					mu.Unlock()
				}
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// Goroutines fazendo Remove
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			localClient, err := rpc.Dial("tcp", ":5000")
			if err != nil {
				return
			}
			defer localClient.Close()

			var localReplyI int
			for j := 0; j < 3; j++ {
				err = localClient.Call("RemoteList.Remove", remotelist.RemoveArgs{ListName: "append_remove_race"}, &localReplyI)
				mu.Lock()
				if err == nil {
					removeOps++
				} else {
					removeErrors++
				}
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	_ = client.Call("RemoteList.Size", remotelist.SizeArgs{ListName: "append_remove_race"}, &reply_i)
	expectedFinalSize := 5 + appendOps - removeOps
	fmt.Printf("Resultado: Appends = %d | Removes = %d | Remove errors = %d | Size final = %d\n",
		appendOps, removeOps, removeErrors, reply_i)
	fmt.Printf("Calculo esperado: 5 (inicial) + %d (appends) - %d (removes) = %d\n",
		appendOps, removeOps, expectedFinalSize)
	if reply_i == expectedFinalSize {
		fmt.Println("Status: PASSOU - Operacoes concorrentes de Append/Remove sao atomicas")
	} else {
		fmt.Println("Status: ATENCAO - Size final nao coincide com esperado (pode haver race conditions ou removes de lista vazia)")
	}

	fmt.Println("\n========================================")
	fmt.Println("TESTES CONCLUIDOS")
	fmt.Println("========================================")
}
